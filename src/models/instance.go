package models

import (
	"fmt"
	"time"
)

//rpc
type InstancesResp struct {
	Data []*Instance
	Msg  string
}

type Instance struct {
	Id       int64  `json:"id"`
	Module   string `json:"module"`
	Identity string `json:"identity"` //ip 或者 机器名
	RPCPort  string `json:"rpc_port" xorm:"rpc_port"`
	HTTPPort string `json:"http_port" xorm:"http_port"`
	TS       int64  `json:"ts" xorm:"ts"`
	Remark   string `json:"remark"`
	Region   string `json:"region"`
	Active   bool   `xorm:"-" json:"active"`
}

func (i *Instance) Add() error {
	_, err := DB["hbs"].InsertOne(i)
	return err
}

func (i *Instance) Update() error {
	_, err := DB["hbs"].Where("id=?", i.Id).MustCols("ts", "http_port", "rpc_port", "region").Update(i)
	return err
}

func GetInstanceBy(mod, identity, rpcPort, httpPort string) (*Instance, error) {
	var obj Instance
	has, err := DB["hbs"].Where("module=? and identity=? and rpc_port=? and http_port=?", mod, identity, rpcPort, httpPort).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func GetAllInstances(mod string, alive int) ([]*Instance, error) {
	objs := make([]*Instance, 0)
	var err error
	now := time.Now().Unix()

	ts := now - 60
	if alive == 1 {
		err = DB["hbs"].Where("module = ? and ts > ?", mod, ts).OrderBy("id").Find(&objs)
	} else {
		err = DB["hbs"].Where("module = ?", mod).OrderBy("id").Find(&objs)
	}
	if err != nil {
		return objs, err
	}
	for _, j := range objs {
		if j.TS > now-60 { //上报心跳时间在1分钟之内
			j.Active = true
		}
	}
	return objs, err
}

func DelById(id int64) error {
	_, err := DB["hbs"].Where("id=?", id).Delete(new(Instance))
	return err
}

func ReportHeartBeat(rev Instance) error {
	instance, err := GetInstanceBy(rev.Module, rev.Identity, rev.RPCPort, rev.HTTPPort)
	if err != nil {
		return fmt.Errorf("get instance:%+v err:%v", rev, err)
	}

	now := time.Now().Unix()
	if instance == nil {
		instance = &Instance{
			Identity: rev.Identity,
			Module:   rev.Module,
			RPCPort:  rev.RPCPort,
			HTTPPort: rev.HTTPPort,
			Region:   rev.Region,
			TS:       now,
		}
		err := instance.Add()
		if err != nil {
			return fmt.Errorf("instance:%+v add err:%v", rev, err)
		}
	} else {
		instance.TS = now
		instance.HTTPPort = rev.HTTPPort
		instance.Region = rev.Region
		err := instance.Update()
		if err != nil {
			return fmt.Errorf("instance:%+v update err:%v", rev, err)
		}
	}
	return nil
}
