package models

import (
	"encoding/json"
	"strings"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"xorm.io/xorm"
)

type NetworkHardwareRpcResp struct {
	Data []*NetworkHardware
	Msg  string
}

type NetworkHardware struct {
	Id          int64  `json:"id"`
	SN          string `json:"sn" xorm:"sn"`
	IP          string `json:"ip" xorm:"ip"`
	Name        string `json:"name"`
	Note        string `json:"note"`
	Cate        string `json:"cate"`
	SnmpVersion string `json:"snmp_version"`
	Auth        string `json:"auth"`
	Region      string `json:"region"`
	Info        string `json:"info"`
	Tenant      string `json:"tenant"`
	Uptime      int64  `json:"uptime"`
}

func MakeNetworkHardware(ip, cate, version, auth, region, note string) *NetworkHardware {
	obj := &NetworkHardware{
		IP:          ip,
		SnmpVersion: version,
		Auth:        auth,
		Region:      region,
		Note:        note,
		Cate:        cate,
	}
	return obj
}

func NetworkHardwareNew(objPtr *NetworkHardware) error {
	session := DB["nems"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	old, err := NetworkHardwareGet("ip=?", objPtr.IP)
	if err != nil {
		session.Rollback()
		return err
	}
	if old != nil {
		session.Rollback()
		return nil
	}

	_, err = session.Insert(objPtr)
	if err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func NetworkHardwareGet(where string, args ...interface{}) (*NetworkHardware, error) {
	var obj NetworkHardware
	has, err := DB["nems"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (n *NetworkHardware) Update(cols ...string) error {
	session := DB["nems"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	_, err := session.Where("id=?", n.Id).Cols(cols...).Update(n)
	if err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

// func (h *Host) Del() error {
// 	_, err := DB["ams"].Where("id=?", h.Id).Delete(new(Host))
// 	return err
// }

func (n *NetworkHardware) Del() error {
	_, err := DB["nems"].Where("id=?", n.Id).Delete(new(NetworkHardware))
	return err
}

func NetworkHardwareCount(where string, args ...interface{}) (int64, error) {
	if where != "" {
		return DB["nems"].Where(where, args...).Count(new(NetworkHardware))
	}

	return DB["nems"].Count(new(NetworkHardware))
}

func NetworkHardwareTotal(query string) (int64, error) {
	return buildHWWhere(query).Count()
}

func NetworkHardwareList(query string, limit, offset int) ([]NetworkHardware, error) {
	session := buildHWWhere(query)
	var objs []NetworkHardware
	err := session.Limit(limit, offset).OrderBy("id desc").Find(&objs)
	return objs, err
}

func buildHWWhere(query string) *xorm.Session {
	session := DB["nems"].Table(new(NetworkHardware))
	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("cate = ? or ip like ? or name like ? or note like ?", arr[i], q, q, q)
		}
	}
	return session
}

func NetworkHardwareDel(id int64) error {
	session := DB["nems"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	var obj NetworkHardware
	has, err := session.Where("id=?", id).Get(&obj)
	if err != nil {
		session.Rollback()
		return err
	}

	if !has {
		return err
	}

	_, err = session.Where("id=?", id).Delete(new(NetworkHardware))
	if err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

// ResourceRegister 资源分配给某个租户的时候调用
func NetworkHardwareResourceRegister(hws []*NetworkHardware, tenant string) error {
	count := len(hws)
	for i := 0; i < count; i++ {
		uuid := hws[i].SN
		res, err := ResourceGet("uuid=?", uuid)
		if err != nil {
			return err
		}

		if res == nil {
			res = &Resource{
				UUID:   uuid,
				Ident:  hws[i].IP,
				Name:   hws[i].Name,
				Cate:   hws[i].Cate,
				Tenant: tenant,
			}

			// 如果host加个字段，并且要放到extend里，这里要改
			fields := map[string]interface{}{
				"region": hws[i].Region,
			}

			js, err := json.Marshal(fields)
			if err != nil {
				return err
			}

			res.Extend = string(js)
			err = res.Save()
			if err != nil {
				return err
			}
		} else {
			if res.Tenant != tenant {
				// 之前有归属，如果归属发生变化，解除之前的挂载关系
				err = NodeResourceUnbindByRids([]int64{res.Id})
				if err != nil {
					return err
				}
			}

			res.Ident = hws[i].IP
			res.Name = hws[i].Name
			res.Cate = hws[i].Cate
			res.Tenant = tenant

			fields := map[string]interface{}{
				"region": hws[i].Region,
			}

			js, err := json.Marshal(fields)
			if err != nil {
				return err
			}

			res.Extend = string(js)
			err = res.Update("ident", "name", "cate", "extend", "tenant")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// NwSearch 普通用户查询
func NwSearch(batch, field string) ([]NetworkHardware, error) {
	arr := str.ParseLines(strings.Replace(batch, ",", "\n", -1))
	if len(arr) == 0 {
		return []NetworkHardware{}, nil
	}

	var objs []NetworkHardware
	err := DB["nems"].Table("network_hardware").In(field, arr).Find(&objs)
	return objs, err
}

func NwTotalForAdmin(tenant, query, batch, field string) (int64, error) {
	return buildNwWhere(tenant, query, batch, field).Count()
}

func NwGetsForAdmin(tenant, query, batch, field string, limit, offset int) ([]NetworkHardware, error) {
	var objs []NetworkHardware
	err := buildNwWhere(tenant, query, batch, field).Limit(limit, offset).Find(&objs)
	return objs, err
}

func buildNwWhere(tenant, query, batch, field string) *xorm.Session {
	session := DB["nems"].Table("network_hardware").OrderBy("id")

	if tenant == "0" {
		session = session.Where("tenant=?", "")
	} else if tenant != "" {
		session = session.Where("tenant=?", tenant)
	}

	if batch == "" && query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("cate=? or sn=? or ip like ? or name like ? or note like ?", arr[i], arr[i], q, q, q)
		}
	}

	if batch != "" {
		arr := str.ParseLines(strings.Replace(batch, ",", "\n", -1))
		if len(arr) > 0 {
			session = session.In(field, arr)
		}
	}

	return session
}

func GetHardwareInfoBy(ips []string) []*NetworkHardware {
	var hws []*NetworkHardware
	for _, ip := range ips {
		hw, err := NetworkHardwareGet("ip=?", ip)
		if err != nil {
			logger.Error(err)
			continue
		}
		hws = append(hws, hw)
	}
	return hws
}
