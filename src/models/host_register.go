package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/logger"
)

type HostRegisterForm struct {
	SN      string                 `json:"sn"`
	IP      string                 `json:"ip"`
	Ident   string                 `json:"ident"`
	Name    string                 `json:"name"`
	Cate    string                 `json:"cate"`
	UniqKey string                 `json:"uniqkey"`
	Fields  map[string]interface{} `json:"fields"`
	Digest  string                 `json:"digest"`
}

func (f HostRegisterForm) Validate() error {
	if f.IP == "" {
		return fmt.Errorf("ip is blank")
	}

	if f.UniqKey == "" {
		return fmt.Errorf("uniqkey is blank")
	}

	if f.Digest == "" {
		return fmt.Errorf("digest is blank")
	}
	return nil
}

// mapKeyClear map key clear
func MapKeyClear(src map[string]interface{}, save map[string]struct{}) {
	var dels []string
	for k := range src {
		if _, ok := save[k]; !ok {
			dels = append(dels, k)
		}
	}

	for i := 0; i < len(dels); i++ {
		delete(src, dels[i])
	}
}

func HostRegister(f HostRegisterForm) error {
	oldFields := make(map[string]interface{}, len(f.Fields))
	for k, v := range f.Fields {
		oldFields[k] = v
	}

	uniqValue := ""

	if f.UniqKey == "sn" {
		uniqValue = f.SN
	}

	if f.UniqKey == "ip" {
		uniqValue = f.IP
	}

	if f.UniqKey == "ident" {
		uniqValue = f.Ident
	}

	if f.UniqKey == "name" {
		uniqValue = f.Name
	}

	if uniqValue == "" {
		return fmt.Errorf("%s is blank", f.UniqKey)
	}

	cacheKey := "/host/info/" + f.UniqKey + "/" + uniqValue

	var val string
	if err := cache.Get(cacheKey, &val); err == nil {
		if f.Digest == val {
			// 说明客户端采集到的各个字段信息并无变化，无需更新DB

			return nil
		}
	} else {
		if err.Error() != cache.ErrCacheMiss.Error() {
			return fmt.Errorf("get cache:%+v err:%v", f, err)
		}
	}

	host, err := HostGet(f.UniqKey+" = ?", uniqValue)
	if err != nil {
		return fmt.Errorf("get host:%+v err:%v", f, err)
	}

	hFixed := map[string]struct{}{
		"cpu":  struct{}{},
		"mem":  struct{}{},
		"disk": struct{}{},
	}

	MapKeyClear(f.Fields, hFixed)

	if host == nil {
		msg := "create host failed"
		host, err = HostNew(f.SN, f.IP, f.Ident, f.Name, f.Cate, f.Fields)
		if err != nil {
			return fmt.Errorf("new host:%+v err:%v", f, err)
		}

		if host == nil {
			return fmt.Errorf("%s, report info:%v", msg, f)
		}
	} else {
		f.Fields["sn"] = f.SN
		f.Fields["ip"] = f.IP
		f.Fields["ident"] = f.Ident
		f.Fields["name"] = f.Name
		f.Fields["cate"] = f.Cate
		f.Fields["clock"] = time.Now().Unix()

		err = host.Update(f.Fields)
		if err != nil {
			return fmt.Errorf("update host:%+v err:%v", f, err)
		}
	}

	if v, ok := oldFields["tenant"]; ok {
		var vStr string
		if reflect.TypeOf(v).String() == "string" {
			vStr = v.(string)
		}

		if vStr != "" {
			err = HostUpdateTenant([]int64{host.Id}, vStr)
			if err != nil {
				return fmt.Errorf("update host:%+v tenant err:%v", f, err)
			}

			err = ResourceRegister([]Host{*host}, vStr)
			if err != nil {
				return fmt.Errorf("resource %+v register err:%v", host, err)
			}
		}
	}

	if host.Tenant != "" {
		// 已经分配给某个租户了，那肯定对应某个resource，需要更新resource的信息
		res, err := ResourceGet("uuid=?", fmt.Sprintf("host-%d", host.Id))
		if err != nil {
			return fmt.Errorf("get resource %v err:%v", host.Id, res)
		}

		if res == nil {
			// 数据不干净，ams里有这个host，而且是已分配状态，但是resource表里没有，重新注册一下
			err := ResourceRegister([]Host{*host}, host.Tenant)
			if err != nil {
				return fmt.Errorf("resource %+v register err:%v", host, err)
			}

			// 注册完了，重新查询一下试试
			res, err = ResourceGet("uuid=?", fmt.Sprintf("host-%d", host.Id))
			if err != nil {
				return fmt.Errorf("get resource %v err:%v", host.Id, res)
			}

			if res == nil {
				return fmt.Errorf("resource %+v register fail, unknown error", host)
			}
		}

		res.Ident = f.Ident
		res.Name = f.Name
		res.Cate = f.Cate

		MapKeyClear(f.Fields, hFixed)

		js, err := json.Marshal(f.Fields)
		if err != nil {
			return fmt.Errorf("json marshal fields:%v err:%v", f.Fields, err)
		}

		res.Extend = string(js)

		err = res.Update("ident", "name", "cate", "extend")
		if err != nil {
			return fmt.Errorf("update err:%v", err)
		}
	}

	var objs []HostFieldValue
	for k, v := range oldFields {
		if k == "tenant" {
			continue
		}

		if _, ok := hFixed[k]; !ok {
			if reflect.TypeOf(v).String() != "string" {
				logger.Debugf("k:%v, v:%v type is not string", k, v)
				continue
			}

			tmp := HostFieldValue{HostId: host.Id, FieldIdent: k, FieldValue: v.(string)}
			objs = append(objs, tmp)
		}
	}

	if len(objs) > 0 {
		err = HostFieldValuePuts(host.Id, objs)
		if err != nil {
			return fmt.Errorf("host:%+v FieldValue %+v Puts err:%v", host, objs, err)
		}
	}

	err = cache.Set(cacheKey, f.Digest, cache.DEFAULT)
	if err != nil {
		return fmt.Errorf("set host:%v cache:%s %v err:%v", f, cacheKey, cache.DEFAULT, err)
	}

	return nil
}
