package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/prober/manager/accumulator"

	"github.com/influxdata/telegraf"
)

type BaseCollector struct {
	name     string
	category Category
	newRule  func() TelegrafPlugin
}

func NewBaseCollector(name string, category Category, newRule func() TelegrafPlugin) *BaseCollector {
	return &BaseCollector{
		name:     name,
		category: category,
		newRule:  newRule,
	}
}

type TelegrafPlugin interface {
	TelegrafInput() (telegraf.Input, error)
}

func (p BaseCollector) Name() string                   { return p.name }
func (p BaseCollector) Category() Category             { return p.category }
func (p BaseCollector) Template() (interface{}, error) { return Template(p.newRule()) }

func (p BaseCollector) TelegrafInput(rule *models.CollectRule) (telegraf.Input, error) {
	r2 := p.newRule()
	if err := json.Unmarshal(rule.Data, r2); err != nil {
		return nil, err
	}

	return r2.TelegrafInput()
}

// used for ui
func (p BaseCollector) Get(id int64) (interface{}, error) {
	collect := &models.CollectRule{}
	has, err := models.DB["mon"].Where("id = ?", id).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
}

func (p BaseCollector) mustGetRule(id int64) (*models.CollectRule, error) {
	collect := &models.CollectRule{}
	has, err := models.DB["mon"].Where("id = ?", id).Get(collect)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("unable to get the collectRule")
	}
	return collect, nil
}

func (p BaseCollector) Gets(nids []int64) (ret []interface{}, err error) {
	collects := []models.CollectRule{}
	err = models.DB["mon"].Where("collect_type=?", p.name).In("nid", nids).Find(&collects)
	for _, c := range collects {
		ret = append(ret, c)
	}
	return ret, err
}

func (p BaseCollector) GetByNameAndNid(name string, nid int64) (interface{}, error) {
	collect := &models.CollectRule{}
	has, err := models.DB["mon"].Where("collect_type = ? and name = ? and nid = ?", p.name, name, nid).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
}

func (p BaseCollector) Create(data []byte, username string) error {
	collect := &models.CollectRule{CollectType: p.name}
	rule := p.newRule()

	if err := json.Unmarshal(data, collect); err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(data), err)
	}

	if err := collect.Validate(rule); err != nil {
		return err
	}

	can, err := models.UsernameCandoNodeOp(username, "mon_collect_create", collect.Nid)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	now := time.Now().Unix()
	collect.Creator = username
	collect.CreatedAt = now
	collect.Updater = username
	collect.UpdatedAt = now

	old, err := p.GetByNameAndNid(collect.Name, collect.Nid)
	if err != nil {
		return err
	}
	if old != nil {
		return fmt.Errorf("同节点下策略名称 %s 已存在", collect.Name)
	}

	if err := models.CreateCollect(p.name, username, collect, collect.DryRun); err != nil {
		return err
	}

	if collect.DryRun {
		return p.dryRun(rule)
	}

	return nil
}

func (p BaseCollector) dryRun(rule TelegrafPlugin) error {
	input, err := rule.TelegrafInput()
	if err != nil {
		return err
	}

	metrics := []*dataobj.MetricValue{}

	acc, err := accumulator.New(accumulator.Options{Name: "plugin-dryrun", Metrics: &metrics})
	if err != nil {
		return err
	}

	if err = input.Gather(acc); err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	for k, v := range metrics {
		fmt.Fprintf(buf, "%d %s %s %f\n", k, v.CounterType, v.PK(), v.Value)
	}
	return NewDryRunError(buf.String())
}

type DryRun struct {
	msg string
}

func (p DryRun) Error() string {
	return p.msg
}

func NewDryRunError(msg string) error {
	return DryRun{msg}
}

func (p BaseCollector) Update(data []byte, username string) error {
	collect := &models.CollectRule{}
	rule := p.newRule()

	if err := json.Unmarshal(data, collect); err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(data), err)
	}

	if err := collect.Validate(rule); err != nil {
		return err
	}

	can, err := models.UsernameCandoNodeOp(username, "mon_collect_modify", collect.Nid)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	//校验采集是否存在
	obj, err := p.mustGetRule(collect.Id)
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.name, collect.Id)
	}

	collect.Updater = username
	collect.UpdatedAt = time.Now().Unix()

	old, err := p.GetByNameAndNid(collect.Name, collect.Nid)
	if err != nil {
		return err
	}
	if old != nil && obj.Id != old.(*models.CollectRule).Id {
		return fmt.Errorf("同节点下策略名称 %s 已存在", collect.Name)
	}

	if err := collect.Update(); err != nil {
		return err
	}

	if collect.DryRun {
		return p.dryRun(rule)
	}

	return nil
}

func (p BaseCollector) Delete(id int64, username string) error {
	rule, err := p.mustGetRule(id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.name, id)
	}
	can, err := models.UsernameCandoNodeOp(username, "mon_collect_delete", int64(rule.Nid))
	if err != nil {
		return fmt.Errorf("models.UsernameCandoNodeOp error %s", err)
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	return models.DeleteCollectRule(id)
}
