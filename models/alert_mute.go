package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/toolkits/pkg/logger"

	"github.com/pkg/errors"
)

type TagFilter struct {
	Key    string              `json:"key"`   // tag key
	Func   string              `json:"func"`  // `==` | `=~` | `in` | `!=` | `!~` | `not in`
	Value  string              `json:"value"` // tag value
	Regexp *regexp.Regexp      // parse value to regexp if func = '=~' or '!~'
	Vset   map[string]struct{} // parse value to regexp if func = 'in' or 'not in'
}

const TimeRange int = 0
const Periodic int = 1

type AlertMute struct {
	Id                int64          `json:"id" gorm:"primaryKey"`
	GroupId           int64          `json:"group_id"`
	Note              string         `json:"note"`
	Cate              string         `json:"cate"`
	Prod              string         `json:"prod"`
	DatasourceIds     string         `json:"-" gorm:"datasource_ids"` // datasource ids
	DatasourceIdsJson []int64        `json:"datasource_ids" gorm:"-"` // for fe
	Cluster           string         `json:"cluster"`                 // take effect by clusters, seperated by space
	Tags              ormx.JSONArr   `json:"tags"`
	Cause             string         `json:"cause"`
	Btime             int64          `json:"btime"`
	Etime             int64          `json:"etime"`
	Disabled          int            `json:"disabled"` // 0: enabled, 1: disabled
	CreateBy          string         `json:"create_by"`
	UpdateBy          string         `json:"update_by"`
	CreateAt          int64          `json:"create_at"`
	UpdateAt          int64          `json:"update_at"`
	ITags             []TagFilter    `json:"-" gorm:"-"`     // inner tags
	MuteTimeType      int            `json:"mute_time_type"` //  0: mute by time range, 1: mute by periodic time
	PeriodicMutes     string         `json:"-" gorm:"periodic_mutes"`
	PeriodicMutesJson []PeriodicMute `json:"periodic_mutes" gorm:"-"`
}

type PeriodicMute struct {
	EnableStime      string `json:"enable_stime"`        // split by space: "00:00 10:00 12:00"
	EnableEtime      string `json:"enable_etime"`        // split by space: "00:00 10:00 12:00"
	EnableDaysOfWeek string `json:"enable_days_of_week"` // eg: "0 1 2 3 4 5 6"
}

func (m *AlertMute) TableName() string {
	return "alert_mute"
}

func AlertMuteGetById(ctx *ctx.Context, id int64) (*AlertMute, error) {
	return AlertMuteGet(ctx, "id=?", id)
}

func AlertMuteGet(ctx *ctx.Context, where string, args ...interface{}) (*AlertMute, error) {
	var lst []*AlertMute
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}
	err = lst[0].DB2FE(ctx)
	return lst[0], err
}

func AlertMuteGets(ctx *ctx.Context, prods []string, bgid int64, query string) (lst []AlertMute, err error) {
	session := DB(ctx).Where("group_id = ? and prod in (?)", bgid, prods)

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("cause like ?", qarg)
		}
	}

	err = session.Order("id desc").Find(&lst).Error
	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE(ctx)
	}
	return
}

func AlertMuteGetsByBG(ctx *ctx.Context, groupId int64) (lst []AlertMute, err error) {
	err = DB(ctx).Where("group_id=?", groupId).Order("id desc").Find(&lst).Error
	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE(ctx)
	}
	return
}

func (m *AlertMute) Verify() error {
	if m.GroupId < 0 {
		return errors.New("group_id invalid")
	}

	if IsAllDatasource(m.DatasourceIdsJson) {
		m.DatasourceIdsJson = []int64{0}
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

func (m *AlertMute) Add(ctx *ctx.Context) error {
	if err := m.Verify(); err != nil {
		return err
	}

	if err := m.FE2DB(); err != nil {
		return err
	}

	now := time.Now().Unix()
	m.CreateAt = now
	m.UpdateAt = now
	return Insert(ctx, m)
}

func (m *AlertMute) Update(ctx *ctx.Context, arm AlertMute) error {

	arm.Id = m.Id
	arm.GroupId = m.GroupId
	arm.CreateAt = m.CreateAt
	arm.CreateBy = m.CreateBy
	arm.UpdateAt = time.Now().Unix()

	err := arm.Verify()
	if err != nil {
		return err
	}

	if err := arm.FE2DB(); err != nil {
		return err
	}

	return DB(ctx).Model(m).Select("*").Updates(arm).Error
}

func (m *AlertMute) FE2DB() error {
	idsBytes, err := json.Marshal(m.DatasourceIdsJson)
	if err != nil {
		return err
	}
	m.DatasourceIds = string(idsBytes)

	periodicMutesBytes, err := json.Marshal(m.PeriodicMutesJson)
	if err != nil {
		return err
	}
	m.PeriodicMutes = string(periodicMutesBytes)

	return nil
}

func (m *AlertMute) DB2FE(ctx *ctx.Context) error {
	json.Unmarshal([]byte(m.DatasourceIds), &m.DatasourceIdsJson)
	err := json.Unmarshal([]byte(m.PeriodicMutes), &m.PeriodicMutesJson)
	return err
}

func (m *AlertMute) UpdateFieldsMap(ctx *ctx.Context, fields map[string]interface{}) error {
	return DB(ctx).Model(m).Updates(fields).Error
}

func AlertMuteDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(AlertMute)).Error
}

func AlertMuteStatistics(ctx *ctx.Context) (*Statistics, error) {
	// clean expired first
	buf := int64(30)
	err := DB(ctx).Where("etime < ? and mute_time_type = 0", time.Now().Unix()-buf).Delete(new(AlertMute)).Error
	if err != nil {
		return nil, err
	}

	session := DB(ctx).Model(&AlertMute{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err = session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func AlertMuteGetsAll(ctx *ctx.Context) ([]*AlertMute, error) {
	// get my cluster's mutes
	session := DB(ctx).Model(&AlertMute{})

	var lst []*AlertMute
	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE(ctx)
	}

	return lst, err
}

func AlertMuteUpgradeToV6(ctx *ctx.Context, dsm map[string]Datasource) error {
	var lst []*AlertMute
	err := DB(ctx).Find(&lst).Error
	if err != nil {
		return err
	}

	for i := 0; i < len(lst); i++ {
		var ids []int64
		if lst[i].Cluster == "$all" {
			ids = append(ids, 0)
		} else {
			clusters := strings.Fields(lst[i].Cluster)
			for j := 0; j < len(clusters); j++ {
				if ds, exists := dsm[clusters[j]]; exists {
					ids = append(ids, ds.Id)
				}
			}
		}

		b, err := json.Marshal(ids)
		if err != nil {
			continue
		}
		lst[i].DatasourceIds = string(b)

		if lst[i].Prod == "" {
			lst[i].Prod = METRIC
		}

		if lst[i].Cate == "" {
			lst[i].Cate = PROMETHEUS
		}

		err = lst[i].UpdateFieldsMap(ctx, map[string]interface{}{
			"datasource_ids": lst[i].DatasourceIds,
			"prod":           lst[i].Prod,
			"cate":           lst[i].Cate,
		})
		if err != nil {
			logger.Errorf("update alert rule:%d datasource ids failed, %v", lst[i].Id, err)
		}
	}
	return nil
}
