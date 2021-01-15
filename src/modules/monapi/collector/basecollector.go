package collector

import (
	"encoding/json"
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/influxdata/telegraf"
)

type BaseCollector struct {
	name     string
	category Category
	newRule  func() interface{}
}

func NewBaseCollector(name string, category Category, newRule func() interface{}) *BaseCollector {
	return &BaseCollector{
		name:     name,
		category: category,
		newRule:  newRule,
	}
}

type telegrafPlugin interface {
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

	plugin, ok := r2.(telegrafPlugin)
	if !ok {
		return nil, errUnsupported
	}

	return plugin.TelegrafInput()
}

func (p BaseCollector) Get(id int64) (interface{}, error) {
	collect := &models.CollectRule{}
	has, err := models.DB["mon"].Where("id = ?", id).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
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

	collect.Creator = username
	collect.LastUpdator = username

	old, err := p.GetByNameAndNid(collect.Name, collect.Nid)
	if err != nil {
		return err
	}
	if old != nil {
		return fmt.Errorf("同节点下策略名称 %s 已存在", collect.Name)
	}
	return models.CreateCollect(p.name, username, collect)
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
	obj, err := p.Get(collect.Id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.name, collect.Id)
	}

	tmpId := obj.(*models.CollectRule).Id
	if tmpId == 0 {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.name, collect.Id)
	}

	collect.Creator = username
	collect.LastUpdator = username

	old, err := p.GetByNameAndNid(collect.Name, collect.Nid)
	if err != nil {
		return err
	}
	if old != nil && tmpId != old.(*models.CollectRule).Id {
		return fmt.Errorf("同节点下策略名称 %s 已存在", collect.Name)
	}

	return collect.Update()
}

func (p BaseCollector) Delete(id int64, username string) error {
	tmp, err := p.Get(id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.name, id)
	}
	nid := tmp.(*models.CollectRule).Nid
	can, err := models.UsernameCandoNodeOp(username, "mon_collect_delete", int64(nid))
	if err != nil {
		return fmt.Errorf("models.UsernameCandoNodeOp error %s", err)
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	return models.DeleteCollectRule(id)
}
