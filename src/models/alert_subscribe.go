package models

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/ormx"
	"github.com/pkg/errors"
)

type AlertSubscribe struct {
	Id               int64        `json:"id" gorm:"primaryKey"`
	Name             string       `json:"name"`     // AlertSubscribe name
	Disabled         int          `json:"disabled"` // 0: enabled, 1: disabled
	GroupId          int64        `json:"group_id"`
	Cate             string       `json:"cate"`
	Cluster          string       `json:"cluster"` // take effect by clusters, seperated by space
	RuleId           int64        `json:"rule_id"`
	RuleName         string       `json:"rule_name" gorm:"-"` // for fe
	Tags             ormx.JSONArr `json:"tags"`
	RedefineSeverity int          `json:"redefine_severity"`
	NewSeverity      int          `json:"new_severity"`
	RedefineChannels int          `json:"redefine_channels"`
	NewChannels      string       `json:"new_channels"`
	UserGroupIds     string       `json:"user_group_ids"`
	UserGroups       []UserGroup  `json:"user_groups" gorm:"-"` // for fe
	CreateBy         string       `json:"create_by"`
	CreateAt         int64        `json:"create_at"`
	UpdateBy         string       `json:"update_by"`
	UpdateAt         int64        `json:"update_at"`
	ITags            []TagFilter  `json:"-" gorm:"-"` // inner tags
}

func (s *AlertSubscribe) TableName() string {
	return "alert_subscribe"
}

func AlertSubscribeGets(groupId int64) (lst []AlertSubscribe, err error) {
	err = DB().Where("group_id=?", groupId).Order("id desc").Find(&lst).Error
	return
}

func AlertSubscribeGet(where string, args ...interface{}) (*AlertSubscribe, error) {
	var lst []*AlertSubscribe
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func (s *AlertSubscribe) IsDisabled() bool {
	return s.Disabled == 1
}

func (s *AlertSubscribe) Verify() error {
	if s.Cluster == "" {
		return errors.New("cluster invalid")
	}

	if IsClusterAll(s.Cluster) {
		s.Cluster = ClusterAll
	}

	if err := s.Parse(); err != nil {
		return err
	}

	if len(s.ITags) == 0 && s.RuleId == 0 {
		return errors.New("rule_id and tags are both blank")
	}

	ugids := strings.Fields(s.UserGroupIds)
	for i := 0; i < len(ugids); i++ {
		if _, err := strconv.ParseInt(ugids[i], 10, 64); err != nil {
			return errors.New("user_group_ids invalid")
		}
	}

	return nil
}

func (s *AlertSubscribe) Parse() error {
	err := json.Unmarshal(s.Tags, &s.ITags)
	if err != nil {
		return err
	}

	for i := 0; i < len(s.ITags); i++ {
		if s.ITags[i].Func == "=~" || s.ITags[i].Func == "!~" {
			s.ITags[i].Regexp, err = regexp.Compile(s.ITags[i].Value)
			if err != nil {
				return err
			}
		} else if s.ITags[i].Func == "in" || s.ITags[i].Func == "not in" {
			arr := strings.Fields(s.ITags[i].Value)
			s.ITags[i].Vset = make(map[string]struct{})
			for j := 0; j < len(arr); j++ {
				s.ITags[i].Vset[arr[j]] = struct{}{}
			}
		}
	}

	return nil
}

func (s *AlertSubscribe) Add() error {
	if err := s.Verify(); err != nil {
		return err
	}

	now := time.Now().Unix()
	s.CreateAt = now
	s.UpdateAt = now
	return Insert(s)
}

func (s *AlertSubscribe) FillRuleName(cache map[int64]string) error {
	if s.RuleId <= 0 {
		s.RuleName = ""
		return nil
	}

	name, has := cache[s.RuleId]
	if has {
		s.RuleName = name
		return nil
	}

	name, err := AlertRuleGetName(s.RuleId)
	if err != nil {
		return err
	}

	if name == "" {
		name = "Error: AlertRule not found"
	}

	s.RuleName = name
	cache[s.RuleId] = name
	return nil
}

func (s *AlertSubscribe) FillUserGroups(cache map[int64]*UserGroup) error {
	// some user-group already deleted ?
	ugids := strings.Fields(s.UserGroupIds)

	count := len(ugids)
	if count == 0 {
		s.UserGroups = []UserGroup{}
		return nil
	}

	exists := make([]string, 0, count)
	delete := false
	for i := range ugids {
		id, _ := strconv.ParseInt(ugids[i], 10, 64)

		ug, has := cache[id]
		if has {
			exists = append(exists, ugids[i])
			s.UserGroups = append(s.UserGroups, *ug)
			continue
		}

		ug, err := UserGroupGetById(id)
		if err != nil {
			return err
		}

		if ug == nil {
			delete = true
		} else {
			exists = append(exists, ugids[i])
			s.UserGroups = append(s.UserGroups, *ug)
			cache[id] = ug
		}
	}

	if delete {
		// some user-group already deleted
		DB().Model(s).Update("user_group_ids", strings.Join(exists, " "))
		s.UserGroupIds = strings.Join(exists, " ")
	}

	return nil
}

func (s *AlertSubscribe) Update(selectField interface{}, selectFields ...interface{}) error {
	if err := s.Verify(); err != nil {
		return err
	}

	return DB().Model(s).Select(selectField, selectFields...).Updates(s).Error
}

func AlertSubscribeDel(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB().Where("id in ?", ids).Delete(new(AlertSubscribe)).Error
}

func AlertSubscribeStatistics(cluster string) (*Statistics, error) {
	session := DB().Model(&AlertSubscribe{}).Select("count(*) as total", "max(update_at) as last_updated")

	if cluster != "" {
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func AlertSubscribeGetsByCluster(cluster string) ([]*AlertSubscribe, error) {
	// get my cluster's subscribes
	session := DB().Model(&AlertSubscribe{})
	if cluster != "" {
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var lst []*AlertSubscribe
	var slst []*AlertSubscribe
	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, s := range lst {
		if MatchCluster(s.Cluster, cluster) {
			slst = append(slst, s)
		}
	}
	return slst, err
}
