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
	Func   string              `json:"func"`  // == | =~ | in
	Value  string              `json:"value"` // tag value
	Regexp *regexp.Regexp      // parse value to regexp if func = '=~'
	Vset   map[string]struct{} // parse value to regexp if func = 'in'
}

type AlertMute struct {
	Id       int64        `json:"id" gorm:"primaryKey"`
	GroupId  int64        `json:"group_id"`
	Cluster  string       `json:"cluster"`
	Tags     ormx.JSONArr `json:"tags"`
	Cause    string       `json:"cause"`
	Btime    int64        `json:"btime"`
	Etime    int64        `json:"etime"`
	CreateBy string       `json:"create_by"`
	CreateAt int64        `json:"create_at"`
	ITags    []TagFilter  `json:"-" gorm:"-"` // inner tags
}

func (m *AlertMute) TableName() string {
	return "alert_mute"
}

func AlertMuteGets(groupId int64) (lst []AlertMute, err error) {
	err = DB().Where("group_id=?", groupId).Order("id desc").Find(&lst).Error
	return
}

func (m *AlertMute) Verify() error {
	if m.GroupId <= 0 {
		return errors.New("group_id invalid")
	}

	if m.Cluster == "" {
		return errors.New("cluster invalid")
	}

	if m.Etime <= m.Btime {
		return fmt.Errorf("Oops... etime(%d) <= btime(%d)", m.Etime, m.Btime)
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
		if m.ITags[i].Func == "=~" {
			m.ITags[i].Regexp, err = regexp.Compile(m.ITags[i].Value)
			if err != nil {
				return err
			}
		} else if m.ITags[i].Func == "in" {
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

func AlertMuteDel(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB().Where("id in ?", ids).Delete(new(AlertMute)).Error
}

func AlertMuteStatistics(cluster string, btime int64) (*Statistics, error) {
	session := DB().Model(&AlertMute{}).Select("count(*) as total", "max(create_at) as last_updated").Where("btime <= ?", btime)

	if cluster != "" {
		session = session.Where("cluster = ?", cluster)
	}

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func AlertMuteGetsByCluster(cluster string, btime int64) ([]*AlertMute, error) {
	// clean expired first
	buf := int64(30)
	err := DB().Where("etime < ?", time.Now().Unix()+buf).Delete(new(AlertMute)).Error
	if err != nil {
		return nil, err
	}

	// get my cluster's mutes
	session := DB().Model(&AlertMute{}).Where("btime <= ?", btime)
	if cluster != "" {
		session = session.Where("cluster = ?", cluster)
	}

	var lst []*AlertMute
	err = session.Find(&lst).Error
	return lst, err
}
