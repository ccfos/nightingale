package models

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type AlertSubscribe struct {
	Id                int64        `json:"id" gorm:"primaryKey"`
	Name              string       `json:"name"`     // AlertSubscribe name
	Disabled          int          `json:"disabled"` // 0: enabled, 1: disabled
	GroupId           int64        `json:"group_id"`
	Prod              string       `json:"prod"`
	Cate              string       `json:"cate"`
	DatasourceIds     string       `json:"-" gorm:"datasource_ids"` // datasource ids
	DatasourceIdsJson []int64      `json:"datasource_ids" gorm:"-"` // for fe
	Cluster           string       `json:"cluster"`                 // take effect by clusters, seperated by space
	RuleId            int64        `json:"rule_id"`
	ForDuration       int64        `json:"for_duration"`       // for duration, unit: second
	RuleName          string       `json:"rule_name" gorm:"-"` // for fe
	Tags              ormx.JSONArr `json:"tags"`
	RedefineSeverity  int          `json:"redefine_severity"`
	NewSeverity       int          `json:"new_severity"`
	RedefineChannels  int          `json:"redefine_channels"`
	NewChannels       string       `json:"new_channels"`
	UserGroupIds      string       `json:"user_group_ids"`
	UserGroups        []UserGroup  `json:"user_groups" gorm:"-"` // for fe
	RedefineWebhooks  int          `json:"redefine_webhooks"`
	Webhooks          string       `json:"-" gorm:"webhooks"`
	WebhooksJson      []string     `json:"webhooks" gorm:"-"`
	CreateBy          string       `json:"create_by"`
	CreateAt          int64        `json:"create_at"`
	UpdateBy          string       `json:"update_by"`
	UpdateAt          int64        `json:"update_at"`
	ITags             []TagFilter  `json:"-" gorm:"-"` // inner tags
}

func (s *AlertSubscribe) TableName() string {
	return "alert_subscribe"
}

func AlertSubscribeGets(ctx *ctx.Context, groupId int64) (lst []AlertSubscribe, err error) {
	err = DB(ctx).Where("group_id=?", groupId).Order("id desc").Find(&lst).Error
	return
}

func AlertSubscribeGetsByService(ctx *ctx.Context) (lst []AlertSubscribe, err error) {
	err = DB(ctx).Find(&lst).Error
	if err != nil {
		return
	}

	for i := range lst {
		lst[i].DB2FE()
	}
	return
}

func AlertSubscribeGet(ctx *ctx.Context, where string, args ...interface{}) (*AlertSubscribe, error) {
	var lst []*AlertSubscribe
	err := DB(ctx).Where(where, args...).Find(&lst).Error
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
	if IsAllDatasource(s.DatasourceIdsJson) {
		s.DatasourceIdsJson = []int64{0}
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

func (s *AlertSubscribe) FE2DB() error {
	idsByte, err := json.Marshal(s.DatasourceIdsJson)
	if err != nil {
		return err
	}
	s.DatasourceIds = string(idsByte)

	if len(s.WebhooksJson) > 0 {
		b, _ := json.Marshal(s.WebhooksJson)
		s.Webhooks = string(b)
	}

	return nil
}

func (s *AlertSubscribe) DB2FE() error {
	if s.DatasourceIds != "" {
		if err := json.Unmarshal([]byte(s.DatasourceIds), &s.DatasourceIdsJson); err != nil {
			return err
		}
	}

	if s.Webhooks != "" {
		if err := json.Unmarshal([]byte(s.Webhooks), &s.WebhooksJson); err != nil {
			return err
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

	return err
}

func (s *AlertSubscribe) Add(ctx *ctx.Context) error {
	if err := s.Verify(); err != nil {
		return err
	}

	if err := s.FE2DB(); err != nil {
		return err
	}

	now := time.Now().Unix()
	s.CreateAt = now
	s.UpdateAt = now
	return Insert(ctx, s)
}

func (s *AlertSubscribe) FillRuleName(ctx *ctx.Context, cache map[int64]string) error {
	if s.RuleId <= 0 {
		s.RuleName = ""
		return nil
	}

	name, has := cache[s.RuleId]
	if has {
		s.RuleName = name
		return nil
	}

	name, err := AlertRuleGetName(ctx, s.RuleId)
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

// for v5 rule
func (s *AlertSubscribe) FillDatasourceIds(ctx *ctx.Context) error {
	if s.DatasourceIds != "" {
		json.Unmarshal([]byte(s.DatasourceIds), &s.DatasourceIdsJson)
		return nil
	}
	return nil
}

func (s *AlertSubscribe) FillUserGroups(ctx *ctx.Context, cache map[int64]*UserGroup) error {
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

		ug, err := UserGroupGetById(ctx, id)
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
		DB(ctx).Model(s).Update("user_group_ids", strings.Join(exists, " "))
		s.UserGroupIds = strings.Join(exists, " ")
	}

	return nil
}

func (s *AlertSubscribe) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if err := s.Verify(); err != nil {
		return err
	}

	if err := s.FE2DB(); err != nil {
		return err
	}

	return DB(ctx).Model(s).Select(selectField, selectFields...).Updates(s).Error
}

func AlertSubscribeDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(AlertSubscribe)).Error
}

func AlertSubscribeStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=alert_subscribe")
		return s, err
	}

	session := DB(ctx).Model(&AlertSubscribe{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func AlertSubscribeGetsAll(ctx *ctx.Context) ([]*AlertSubscribe, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*AlertSubscribe](ctx, "/v1/n9e/alert-subscribes")
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(lst); i++ {
			lst[i].FE2DB()
		}
		return lst, err
	}

	// get my cluster's subscribes
	session := DB(ctx).Model(&AlertSubscribe{})

	var lst []*AlertSubscribe
	err := session.Find(&lst).Error
	return lst, err
}

func (s *AlertSubscribe) MatchCluster(dsId int64) bool {
	// 没有配置数据源, 或者事件不需要关联数据源
	// do not match any datasource or event not related to datasource
	if len(s.DatasourceIdsJson) == 0 || dsId == 0 {
		return true
	}

	for _, id := range s.DatasourceIdsJson {
		if id == dsId || id == 0 {
			return true
		}
	}
	return false
}

func (s *AlertSubscribe) ModifyEvent(event *AlertCurEvent) {
	if s.RedefineSeverity == 1 {
		event.Severity = s.NewSeverity
	}

	if s.RedefineChannels == 1 {
		event.NotifyChannels = s.NewChannels
		event.NotifyChannelsJSON = strings.Fields(s.NewChannels)
	}

	if s.RedefineWebhooks == 1 {
		event.Callbacks = s.Webhooks
		event.CallbacksJSON = s.WebhooksJson
	}

	event.NotifyGroups = s.UserGroupIds
	event.NotifyGroupsJSON = strings.Fields(s.UserGroupIds)
}

func (s *AlertSubscribe) UpdateFieldsMap(ctx *ctx.Context, fields map[string]interface{}) error {
	return DB(ctx).Model(s).Updates(fields).Error
}

func AlertSubscribeUpgradeToV6(ctx *ctx.Context, dsm map[string]Datasource) error {
	var lst []*AlertSubscribe
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
			"cate":           PROMETHEUS,
		})
		if err != nil {
			logger.Errorf("update alert rule:%d datasource ids failed, %v", lst[i].Id, err)
		}
	}
	return nil
}
