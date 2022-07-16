package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
)

// A RecordingRule records its vector expression into new timeseries.
type RecordingRule struct {
	Id               int64    `json:"id" gorm:"primaryKey"`
	GroupId          int64    `json:"group_id"`             // busi group id
	Cluster          string   `json:"cluster"`              // take effect by cluster, seperated by space
	Name             string   `json:"name"`                 // new metric name
	Note             string   `json:"note"`                 // note
	Disabled         int      `json:"disabled"`             // 0: enabled, 1: disabled
	PromQl           string   `json:"prom_ql"`              // just one ql for promql
	PromEvalInterval int      `json:"prom_eval_interval"`   // unit:s
	AppendTags       string   `json:"-"`                    // split by space: service=n9e mod=api
	AppendTagsJSON   []string `json:"append_tags" gorm:"-"` // for fe
	CreateAt         int64    `json:"create_at"`
	CreateBy         string   `json:"create_by"`
	UpdateAt         int64    `json:"update_at"`
	UpdateBy         string   `json:"update_by"`
}

func (re *RecordingRule) TableName() string {
	return "recording_rule"
}

func (re *RecordingRule) FE2DB() {
	//re.Cluster = strings.Join(re.ClusterJSON, " ")
	re.AppendTags = strings.Join(re.AppendTagsJSON, " ")
}

func (re *RecordingRule) DB2FE() {
	//re.ClusterJSON = strings.Fields(re.Cluster)
	re.AppendTagsJSON = strings.Fields(re.AppendTags)
}

func (re *RecordingRule) Verify() error {
	if re.GroupId < 0 {
		return fmt.Errorf("GroupId(%d) invalid", re.GroupId)
	}

	if re.Cluster == "" {
		return errors.New("cluster is blank")
	}

	if IsClusterAll(re.Cluster) {
		re.Cluster = ClusterAll
	}

	if !model.MetricNameRE.MatchString(re.Name) {
		return errors.New("Name has invalid chreacters")
	}

	if re.Name == "" {
		return errors.New("name is blank")
	}

	if re.PromEvalInterval <= 0 {
		re.PromEvalInterval = 60
	}

	re.AppendTags = strings.TrimSpace(re.AppendTags)
	rer := strings.Fields(re.AppendTags)
	for i := 0; i < len(rer); i++ {
		pair := strings.Split(rer[i], "=")
		if len(pair) != 2 || !model.LabelNameRE.MatchString(pair[0]) {
			return fmt.Errorf("AppendTags(%s) invalid", rer[i])
		}
	}

	return nil
}

func (re *RecordingRule) Add() error {
	if err := re.Verify(); err != nil {
		return err
	}

	exists, err := RecordingRuleExists(0, re.GroupId, re.Cluster, re.Name)
	if err != nil {
		return err
	}

	if exists {
		return errors.New("RecordingRule already exists")
	}

	now := time.Now().Unix()
	re.CreateAt = now
	re.UpdateAt = now

	return Insert(re)
}

func (re *RecordingRule) Update(ref RecordingRule) error {
	if re.Name != ref.Name {
		exists, err := RecordingRuleExists(re.Id, re.GroupId, re.Cluster, ref.Name)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("RecordingRule already exists")
		}
	}

	ref.FE2DB()
	ref.Id = re.Id
	ref.GroupId = re.GroupId
	ref.CreateAt = re.CreateAt
	ref.CreateBy = re.CreateBy
	ref.UpdateAt = time.Now().Unix()
	err := ref.Verify()
	if err != nil {
		return err
	}
	return DB().Model(re).Select("*").Updates(ref).Error
}

func (re *RecordingRule) UpdateFieldsMap(fields map[string]interface{}) error {
	return DB().Model(re).Updates(fields).Error
}

func RecordingRuleDels(ids []int64, groupId int64) error {
	for i := 0; i < len(ids); i++ {
		ret := DB().Where("id = ? and group_id=?", ids[i], groupId).Delete(&RecordingRule{})
		if ret.Error != nil {
			return ret.Error
		}
	}

	return nil
}

func RecordingRuleExists(id, groupId int64, cluster, name string) (bool, error) {
	session := DB().Where("id <> ? and group_id = ? and name =? ", id, groupId, name)

	var lst []RecordingRule
	err := session.Find(&lst).Error
	if err != nil {
		return false, err
	}
	if len(lst) == 0 {
		return false, nil
	}

	// match cluster
	for _, r := range lst {
		if MatchCluster(r.Cluster, cluster) {
			return true, nil
		}
	}
	return false, nil
}

func RecordingRuleGets(groupId int64) ([]RecordingRule, error) {
	session := DB().Where("group_id=?", groupId).Order("name")

	var lst []RecordingRule
	err := session.Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func RecordingRuleGet(where string, regs ...interface{}) (*RecordingRule, error) {
	var lst []*RecordingRule
	err := DB().Where(where, regs...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].DB2FE()

	return lst[0], nil
}

func RecordingRuleGetById(id int64) (*RecordingRule, error) {
	return RecordingRuleGet("id=?", id)
}

func RecordingRuleGetsByCluster(cluster string) ([]*RecordingRule, error) {
	session := DB().Where("disabled = ?", 0)

	if cluster != "" {
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var lst []*RecordingRule
	err := session.Find(&lst).Error
	if err != nil {
		return lst, err
	}

	if len(lst) == 0 {
		return lst, nil
	}

	if cluster == "" {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
		return lst, nil
	}

	lr := make([]*RecordingRule, 0, len(lst))
	for _, r := range lst {
		if MatchCluster(r.Cluster, cluster) {
			r.DB2FE()
			lr = append(lr, r)
		}
	}

	return lr, err
}

func RecordingRuleStatistics(cluster string) (*Statistics, error) {
	session := DB().Model(&RecordingRule{}).Select("count(*) as total", "max(update_at) as last_updated")
	if cluster != "" {
		// 简略的判断，当一个clustername是另一个clustername的substring的时候，会出现stats与预期不符，不影响使用
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}
