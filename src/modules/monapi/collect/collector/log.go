package collector

import (
	"encoding/json"
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collect"
)

func init() {
	collect.CollectorRegister(&LogCollector{})
}

type LogCollector struct{}

func (p LogCollector) Name() string                   { return "log" }
func (p LogCollector) Category() collect.Category     { return collect.LocalCategory }
func (p LogCollector) Template() (interface{}, error) { return nil, nil }

func (p LogCollector) Get(id int64) (interface{}, error) {
	collect := new(models.LogCollect)
	has, err := models.DB["mon"].Where("id = ?", id).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
}

func (p LogCollector) Gets(nids []int64) (ret []interface{}, err error) {
	collects := []models.LogCollect{}
	err = models.DB["mon"].In("nid", nids).Find(&collects)
	for _, c := range collects {
		c.Decode()
		ret = append(ret, c)
	}
	return ret, err
}

func (p LogCollector) GetByNameAndNid(name string, nid int64) (interface{}, error) {
	collect := new(models.LogCollect)
	has, err := models.DB["mon"].Where("name = ? and nid = ?", name, nid).Get(collect)
	if !has {
		return nil, err
	}
	return collect, err
}

func (p LogCollector) Create(data []byte, username string) error {
	collect := new(models.LogCollect)

	err := json.Unmarshal(data, collect)
	if err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(data), err)
	}

	can, err := models.UsernameCandoNodeOp(username, "mon_collect_create", collect.Nid)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	collect.Encode()
	collect.Creator = username
	collect.LastUpdator = username

	nid := collect.Nid
	name := collect.Name

	old, err := p.GetByNameAndNid(name, nid)
	if err != nil {
		return err
	}
	if old != nil {
		return fmt.Errorf("同节点下策略名称 %s 已存在", name)
	}
	return models.CreateCollect(p.Name(), username, collect)
}

func (p LogCollector) Update(data []byte, username string) error {
	collect := new(models.LogCollect)

	err := json.Unmarshal(data, collect)
	if err != nil {
		return fmt.Errorf("unmarshal body %s err:%v", string(data), err)
	}

	can, err := models.UsernameCandoNodeOp(username, "mon_collect_modify", collect.Nid)
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	collect.Encode()
	nid := collect.Nid
	name := collect.Name

	//校验采集是否存在
	obj, err := p.Get(collect.Id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.Name(), collect.Id)
	}

	tmpId := obj.(*models.LogCollect).Id
	if tmpId == 0 {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.Name(), collect.Id)
	}

	collect.Creator = username
	collect.LastUpdator = username

	old, err := p.GetByNameAndNid(name, nid)
	if err != nil {
		return err
	}
	if old != nil && tmpId != old.(*models.LogCollect).Id {
		return fmt.Errorf("同节点下策略名称 %s 已存在", name)
	}

	return collect.Update()
}

func (p LogCollector) Delete(id int64, username string) error {
	tmp, err := p.Get(id) //id找不到的情况
	if err != nil {
		return fmt.Errorf("采集不存在 type:%s id:%d", p.Name(), id)
	}
	nid := tmp.(*models.LogCollect).Nid
	can, err := models.UsernameCandoNodeOp(username, "mon_collect_delete", int64(nid))
	if err != nil {
		return err
	}
	if !can {
		return fmt.Errorf("permission deny")
	}

	return models.DeleteCollectById(p.Name(), username, id)
}
