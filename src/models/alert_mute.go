package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/ormx"
	"github.com/pkg/errors"
)

type TagFilter struct {
	Key    string              `json:"key"`   // tag key
	Func   string              `json:"func"`  // `==` | `=~` | `in` | `!=` | `!~` | `not in`
	Value  string              `json:"value"` // tag value
	Regexp *regexp.Regexp      // parse value to regexp if func = '=~' or '!~'
	Vset   map[string]struct{} // parse value to regexp if func = 'in' or 'not in'
}

type AlertMute struct {
	Id       int64        `json:"id" gorm:"primaryKey"`
	GroupId  int64        `json:"group_id"`
	Note     string       `json:"note"`
	Cate     string       `json:"cate"`
	Prod     string       `json:"prod"`    // product empty means n9e
	Cluster  string       `json:"cluster"` // take effect by clusters, seperated by space
	Tags     ormx.JSONArr `json:"tags"`
	Cause    string       `json:"cause"`
	Btime    int64        `json:"btime"`
	Etime    int64        `json:"etime"`
	Disabled int          `json:"disabled"` // 0: enabled, 1: disabled
	CreateBy string       `json:"create_by"`
	UpdateBy string       `json:"update_by"`
	CreateAt int64        `json:"create_at"`
	UpdateAt int64        `json:"update_at"`
	ITags    []TagFilter  `json:"-" gorm:"-"` // inner tags
}

func (m *AlertMute) TableName() string {
	return "alert_mute"
}

func AlertMuteGetById(id int64) (*AlertMute, error) {
	return AlertMuteGet("id=?", id)
}

func AlertMuteGet(where string, args ...interface{}) (*AlertMute, error) {
	var lst []*AlertMute
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0], nil
}

func AlertMuteGets(prods []string, bgid int64, query string) (lst []AlertMute, err error) {
	session := DB().Where("group_id = ? and prod in (?)", bgid, prods)

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("cause like ?", qarg)
		}
	}

	err = session.Order("id desc").Find(&lst).Error
	return
}

func AlertMuteGetsByBG(groupId int64) (lst []AlertMute, err error) {
	err = DB().Where("group_id=?", groupId).Order("id desc").Find(&lst).Error
	return
}

func (m *AlertMute) Verify() error {
	if m.GroupId < 0 {
		return errors.New("group_id invalid")
	}

	if m.Cluster == "" {
		return errors.New("cluster invalid")
	}

	if IsClusterAll(m.Cluster) {
		m.Cluster = ClusterAll
	}

	if m.Etime <= m.Btime {
		return fmt.Errorf("oops... etime(%d) <= btime(%d)", m.Etime, m.Btime)
	}

	if err := m.Parse(); err != nil {
		return err
	}

	if len(m.ITags) == 0 {
		return errors.New("tags is blank")
	}

	return nil
}

func (m *AlertMute) Parse() error {
	err := json.Unmarshal(m.Tags, &m.ITags)
	if err != nil {
		return err
	}

	for i := 0; i < len(m.ITags); i++ {
		if m.ITags[i].Func == "=~" || m.ITags[i].Func == "!~" {
			m.ITags[i].Regexp, err = regexp.Compile(m.ITags[i].Value)
			if err != nil {
				return err
			}
		} else if m.ITags[i].Func == "in" || m.ITags[i].Func == "not in" {
			arr := strings.Fields(m.ITags[i].Value)
			m.ITags[i].Vset = make(map[string]struct{})
			for j := 0; j < len(arr); j++ {
				m.ITags[i].Vset[arr[j]] = struct{}{}
			}
		}
	}

	return nil
}

func (m *AlertMute) Add() error {
	if err := m.Verify(); err != nil {
		return err
	}
	m.CreateAt = time.Now().Unix()
	return Insert(m)
}

func (m *AlertMute) Update(arm AlertMute) error {

	arm.Id = m.Id
	arm.GroupId = m.GroupId
	arm.CreateAt = m.CreateAt
	arm.CreateBy = m.CreateBy
	arm.UpdateAt = time.Now().Unix()

	err := arm.Verify()
	if err != nil {
		return err
	}
	return DB().Model(m).Select("*").Updates(arm).Error
}

func (m *AlertMute) UpdateFieldsMap(fields map[string]interface{}) error {
	return DB().Model(m).Updates(fields).Error
}

func AlertMuteDel(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB().Where("id in ?", ids).Delete(new(AlertMute)).Error
}

func AlertMuteStatistics(cluster string) (*Statistics, error) {
	// clean expired first
	buf := int64(30)
	err := DB().Where("etime < ?", time.Now().Unix()-buf).Delete(new(AlertMute)).Error
	if err != nil {
		return nil, err
	}

	session := DB().Model(&AlertMute{}).Select("count(*) as total", "max(update_at) as last_updated")
	if cluster != "" {
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var stats []*Statistics
	err = session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func AlertMuteGetsByCluster(cluster string) ([]*AlertMute, error) {
	// get my cluster's mutes
	session := DB().Model(&AlertMute{})
	if cluster != "" {
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var lst []*AlertMute
	var mlst []*AlertMute
	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if cluster == "" {
		return lst, nil
	}

	for _, m := range lst {
		if MatchCluster(m.Cluster, cluster) {
			mlst = append(mlst, m)
		}
	}
	return mlst, err
}
