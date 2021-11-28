package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
)

type CollectRule struct {
	Id               int64    `json:"id"`
	GroupId          int64    `json:"group_id"`
	Cluster          string   `json:"cluster"`
	TargetIdents     string   `json:"-"`
	TargetIdentsJSON []string `json:"target_idents" gorm:"-"`
	TargetTags       string   `json:"-"`
	TargetTagsJSON   []string `json:"target_tags" gorm:"-"`
	Name             string   `json:"name"`
	Note             string   `json:"note"`
	Step             int      `json:"step"`
	Type             string   `json:"type"`
	Data             string   `json:"data"`
	AppendTags       string   `json:"-"`
	AppendTagsJSON   []string `json:"append_tags" gorm:"-"`
	CreateAt         int64    `json:"create_at"`
	CreateBy         string   `json:"create_by"`
	UpdateAt         int64    `json:"update_at"`
	UpdateBy         string   `json:"update_by"`
}

type PortConfig struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // tcp or udp
	Timeout  int    `json:"timeout"`  // second
}

type ProcConfig struct {
	Method string `json:"method"`
	Param  string `json:"param"`
}

type ScriptConfig struct {
	Path    string            `json:"path"`
	Params  string            `json:"params"`
	Stdin   string            `json:"stdin"`
	Env     map[string]string `json:"env"`
	Timeout int               `json:"timeout"` // second
}

type LogConfig struct {
	FilePath    string            `json:"file_path"`
	Func        string            `json:"func"`
	Pattern     string            `json:"pattern"`
	TagsPattern map[string]string `json:"tags_pattern"`
}

func (cr *CollectRule) TableName() string {
	return "collect_rule"
}

func (cr *CollectRule) FE2DB() {
	cr.TargetIdents = strings.Join(cr.TargetIdentsJSON, " ")
	cr.TargetTags = strings.Join(cr.TargetTagsJSON, " ")
	cr.AppendTags = strings.Join(cr.AppendTagsJSON, " ")
}

func (cr *CollectRule) DB2FE() {
	cr.TargetIdentsJSON = strings.Fields(cr.TargetIdents)
	cr.TargetTagsJSON = strings.Fields(cr.TargetTags)
	cr.AppendTagsJSON = strings.Fields(cr.AppendTags)
}

func (cr *CollectRule) Verify() error {
	if str.Dangerous(cr.Name) {
		return errors.New("Name has invalid characters")
	}

	if cr.TargetIdents == "" && cr.TargetTags == "" {
		return errors.New("target_idents and target_tags are both blank")
	}

	if cr.Step <= 0 {
		cr.Step = 15
	}

	if cr.Cluster == "" {
		return errors.New("cluster is blank")
	}

	switch cr.Type {
	case "port":
		var conf PortConfig
		err := json.Unmarshal([]byte(cr.Data), &conf)
		if err != nil {
			return err
		}
	case "script":
		var conf ScriptConfig
		err := json.Unmarshal([]byte(cr.Data), &conf)
		if err != nil {
			return err
		}
	case "log":
		var conf LogConfig
		err := json.Unmarshal([]byte(cr.Data), &conf)
		if err != nil {
			return err
		}
	case "process":
		var conf ProcConfig
		err := json.Unmarshal([]byte(cr.Data), &conf)
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported type")
	}

	return nil
}

func CollectRuleDels(ids []int64, busiGroupId int64) error {
	return DB().Where("id in ? and group_id=?", ids, busiGroupId).Delete(&CollectRule{}).Error
}

func CollectRuleExists(where string, args ...interface{}) (bool, error) {
	return Exists(DB().Model(&CollectRule{}).Where(where, args...))
}

func CollectRuleGets(groupId int64, typ string) ([]CollectRule, error) {
	session := DB().Where("group_id=?", groupId).Order("name")

	if typ != "" {
		session = session.Where("type = ?", typ)
	}

	var lst []CollectRule
	err := session.Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func CollectRuleGet(where string, args ...interface{}) (*CollectRule, error) {
	var lst []*CollectRule
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].DB2FE()

	return lst[0], nil
}

func CollectRuleGetById(id int64) (*CollectRule, error) {
	return CollectRuleGet("id=?", id)
}

func (cr *CollectRule) Add() error {
	if err := cr.Verify(); err != nil {
		return err
	}

	exists, err := CollectRuleExists("group_id=? and type=? and name=? and cluster=?", cr.GroupId, cr.Type, cr.Name, cr.Cluster)
	if err != nil {
		return err
	}

	if exists {
		return errors.New("CollectRule already exists")
	}

	now := time.Now().Unix()
	cr.CreateAt = now
	cr.UpdateAt = now

	return Insert(cr)
}

func (cr *CollectRule) Update(crf CollectRule) error {
	if cr.Name != crf.Name {
		exists, err := CollectRuleExists("group_id=? and type=? and name=? and id <> ? and cluster=?", cr.GroupId, cr.Type, crf.Name, cr.Id, cr.Cluster)
		if err != nil {
			return err
		}

		if exists {
			return errors.New("CollectRule already exists")
		}
	}

	crf.FE2DB()
	crf.Id = cr.Id
	crf.GroupId = cr.GroupId
	crf.Type = cr.Type
	crf.CreateAt = cr.CreateAt
	crf.CreateBy = cr.CreateBy
	crf.UpdateAt = time.Now().Unix()

	return DB().Model(cr).Select("*").Updates(crf).Error
}
