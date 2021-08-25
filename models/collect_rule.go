package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type CollectRule struct {
	Id          int64  `json:"id"`
	ClasspathId int64  `json:"classpath_id"`
	PrefixMatch int    `json:"prefix_match"`
	Name        string `json:"name"`
	Note        string `json:"note"`
	Step        int    `json:"step"`
	Type        string `json:"type"`
	Data        string `json:"data"`
	AppendTags  string `json:"append_tags"`
	CreateAt    int64  `json:"create_at"`
	CreateBy    string `json:"create_by"`
	UpdateAt    int64  `json:"update_at"`
	UpdateBy    string `json:"update_by"`
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

func (cr *CollectRule) Validate() error {
	if str.Dangerous(cr.Name) {
		return _e("CollectRule name has invalid characters")
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
	}

	return nil
}

func (cr *CollectRule) Add() error {
	now := time.Now().Unix()
	cr.CreateAt = now
	cr.UpdateAt = now
	err := cr.Validate()
	if err != nil {
		return err
	}

	return DBInsertOne(cr)
}

func (cr *CollectRule) Del() error {
	_, err := DB.Where("id=?", cr.Id).Delete(new(CollectRule))
	if err != nil {
		logger.Errorf("mysql.error: delete collect_rule(id=%d) fail: %v", cr.Id, err)
		return internalServerError
	}
	return nil
}

func (cr *CollectRule) Update(cols ...string) error {
	err := cr.Validate()
	if err != nil {
		return err
	}

	_, err = DB.Where("id=?", cr.Id).Cols(cols...).Update(cr)
	if err != nil {
		logger.Errorf("mysql.error: update collect_rule(id=%d) fail: %v", cr.Id, err)
		return internalServerError
	}

	return nil
}

func CollectRuleCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(CollectRule))
	if err != nil {
		logger.Errorf("mysql.error: count collect_rule fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func CollectRuleGet(where string, args ...interface{}) (*CollectRule, error) {
	var obj CollectRule
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query collect_rule(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, err
}

// CollectRuleGets 量不大，前端检索和排序
func CollectRuleGets(where string, args ...interface{}) ([]CollectRule, error) {
	var objs []CollectRule
	err := DB.Where(where, args...).OrderBy("name").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: get all collect_rule fail: %v", err)
		return nil, internalServerError
	}

	return objs, nil
}

func CollectRuleGetAll() ([]*CollectRule, error) {
	var objs []*CollectRule
	err := DB.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: get all collect_rule fail: %v", err)
		return nil, internalServerError
	}

	return objs, nil
}

func CollectRulesDel(ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("param ids is empty")
	}

	_, err := DB.Exec("DELETE FROM collect_rule where id in (" + str.IdsString(ids) + ")")
	if err != nil {
		logger.Errorf("mysql.error: delete collect_rule(%v) fail: %v", ids, err)
		return internalServerError
	}

	return nil
}
