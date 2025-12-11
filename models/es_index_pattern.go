package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/pkg/errors"
)

type EsIndexPattern struct {
	Id                         int64  `json:"id" gorm:"primaryKey"`
	DatasourceId               int64  `json:"datasource_id"`
	Name                       string `json:"name"`
	TimeField                  string `json:"time_field"`
	AllowHideSystemIndices     int    `json:"-" gorm:"allow_hide_system_indices"`
	AllowHideSystemIndicesBool bool   `json:"allow_hide_system_indices" gorm:"-"`
	FieldsFormat               string `json:"fields_format"`
	CreateAt                   int64  `json:"create_at"`
	CreateBy                   string `json:"create_by"`
	UpdateAt                   int64  `json:"update_at"`
	UpdateBy                   string `json:"update_by"`
	CrossClusterEnabled        int    `json:"cross_cluster_enabled"`
	Note                       string `json:"note"`
}

func (t *EsIndexPattern) TableName() string {
	return "es_index_pattern"
}

func (r *EsIndexPattern) Add(ctx *ctx.Context) error {
	esIndexPattern, err := EsIndexPatternGet(ctx, "datasource_id = ? and name = ?", r.DatasourceId, r.Name)
	if err != nil {
		return errors.WithMessage(err, "failed to query es index pattern")
	}

	if esIndexPattern != nil {
		return errors.New("es index pattern datasource and name already exists")
	}

	r.FE2DB()

	return Insert(ctx, r)
}

func EsIndexPatternDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	// 检查是否有告警规则引用了这些 index pattern
	for _, id := range ids {
		alertRules, err := GetAlertRulesByEsIndexPatternId(ctx, id)
		if err != nil {
			return errors.WithMessage(err, "failed to check alert rules")
		}
		if len(alertRules) > 0 {
			names := make([]string, 0, len(alertRules))
			for _, rule := range alertRules {
				names = append(names, rule.Name)
			}
			return errors.Errorf("index pattern(id=%d) is used by alert rules: %s", id, strings.Join(names, ", "))
		}
	}

	return DB(ctx).Where("id in ?", ids).Delete(new(EsIndexPattern)).Error
}

// GetAlertRulesByEsIndexPatternId 获取引用了指定 index pattern 的告警规则
func GetAlertRulesByEsIndexPatternId(ctx *ctx.Context, indexPatternId int64) ([]*AlertRule, error) {
	// index_pattern 存储在 rule_config JSON 字段的 queries 数组中
	// 格式如: {"queries":[{"index_type":"index_pattern","index_pattern":123,...}]}
	// 先用 LIKE 粗筛，再在代码中精确过滤
	pattern := fmt.Sprintf(`%%"index_pattern":%d%%`, indexPatternId)

	var candidates []*AlertRule
	err := DB(ctx).Where("rule_config LIKE ?", pattern).Find(&candidates).Error
	if err != nil {
		return nil, err
	}

	// 精确过滤：解析 JSON 检查 index_pattern 字段值是否精确匹配
	var alertRules []*AlertRule
	for _, rule := range candidates {
		if ruleUsesIndexPattern(rule.RuleConfig, indexPatternId) {
			alertRules = append(alertRules, rule)
		}
	}

	return alertRules, nil
}

// ruleUsesIndexPattern 检查告警规则的 rule_config 是否引用了指定的 index_pattern
func ruleUsesIndexPattern(ruleConfig string, indexPatternId int64) bool {
	var config struct {
		Queries []struct {
			IndexPattern int64 `json:"index_pattern"`
		} `json:"queries"`
	}

	if err := json.Unmarshal([]byte(ruleConfig), &config); err != nil {
		return false
	}

	for _, query := range config.Queries {
		if query.IndexPattern == indexPatternId {
			return true
		}
	}

	return false
}

func (ei *EsIndexPattern) Update(ctx *ctx.Context, eip EsIndexPattern) error {
	if ei.Name != eip.Name || ei.DatasourceId != eip.DatasourceId {
		exists, err := EsIndexPatternExists(ctx, ei.Id, eip.DatasourceId, eip.Name)
		if err != nil {
			return err
		}

		if exists {
			return errors.New("EsIndexPattern already exists")
		}
	}

	eip.Id = ei.Id
	eip.CreateAt = ei.CreateAt
	eip.CreateBy = ei.CreateBy
	eip.UpdateAt = time.Now().Unix()

	eip.FE2DB()

	return DB(ctx).Model(ei).Select("*").Updates(eip).Error
}

func (dbIndexPattern *EsIndexPattern) DB2FE() {
	if dbIndexPattern.AllowHideSystemIndices == 1 {
		dbIndexPattern.AllowHideSystemIndicesBool = true
	}
}

func (feIndexPattern *EsIndexPattern) FE2DB() {
	if feIndexPattern.AllowHideSystemIndicesBool {
		feIndexPattern.AllowHideSystemIndices = 1
	}
}

func EsIndexPatternGets(ctx *ctx.Context, where string, args ...interface{}) ([]*EsIndexPattern, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*EsIndexPattern](ctx, "/v1/n9e/es-index-pattern-list")
		return lst, err
	}
	var objs []*EsIndexPattern
	err := DB(ctx).Where(where, args...).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to query es index pattern")
	}

	for _, i := range objs {
		i.DB2FE()
	}
	return objs, nil
}

func EsIndexPatternGet(ctx *ctx.Context, where string, args ...interface{}) (*EsIndexPattern, error) {
	var lst []*EsIndexPattern
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].DB2FE()

	return lst[0], nil
}

func EsIndexPatternGetById(ctx *ctx.Context, id int64) (*EsIndexPattern, error) {
	return EsIndexPatternGet(ctx, "id=?", id)
}

func EsIndexPatternExists(ctx *ctx.Context, id, datasourceId int64, name string) (bool, error) {
	session := DB(ctx).Where("id <> ? and datasource_id = ? and name = ?", id, datasourceId, name)

	var lst []EsIndexPattern
	err := session.Find(&lst).Error
	if err != nil {
		return false, err
	}
	if len(lst) == 0 {
		return false, nil
	}

	return true, nil
}
