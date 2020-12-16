package models

import (
	"strings"
	"time"

	"xorm.io/xorm"

	"github.com/toolkits/pkg/str"
)

type Host struct {
	Id     int64  `json:"id"`
	SN     string `json:"sn" xorm:"'sn'"`
	IP     string `json:"ip" xorm:"'ip'"`
	Ident  string `json:"ident"`
	Name   string `json:"name"`
	Note   string `json:"note"`
	CPU    string `json:"cpu" xorm:"'cpu'"`
	Mem    string `json:"mem"`
	Disk   string `json:"disk"`
	Cate   string `json:"cate"`
	Clock  int64  `json:"clock"`
	Tenant string `json:"tenant"`
}

func (h *Host) Save() error {
	_, err := DB["ams"].Insert(h)
	return err
}

func HostNew(sn, ip, ident, name, cate string, fields map[string]interface{}) (*Host, error) {
	host := new(Host)
	host.SN = sn
	host.IP = ip
	host.Ident = ident
	host.Name = name
	host.Cate = cate
	host.Clock = time.Now().Unix()

	session := DB["ams"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return nil, err
	}

	if _, err := session.Insert(host); err != nil {
		session.Rollback()
		return nil, err
	}

	if len(fields) > 0 {
		if _, err := session.Table(new(Host)).ID(host.Id).Update(fields); err != nil {
			session.Rollback()
			return nil, err
		}
	}

	err := session.Commit()

	return host, err
}

func (h *Host) Update(fields map[string]interface{}) error {
	_, err := DB["ams"].Table(new(Host)).ID(h.Id).Update(fields)
	return err
}

func (h *Host) Del() error {
	_, err := DB["ams"].Where("id=?", h.Id).Delete(new(Host))
	return err
}

func HostUpdateNote(ids []int64, note string) error {
	_, err := DB["ams"].Exec("UPDATE host SET note=? WHERE id in ("+str.IdsString(ids)+")", note)
	return err
}

func HostUpdateCate(ids []int64, cate string) error {
	_, err := DB["ams"].Exec("UPDATE host SET cate=? WHERE id in ("+str.IdsString(ids)+")", cate)
	return err
}

func HostUpdateTenant(ids []int64, tenant string) error {
	_, err := DB["ams"].Exec("UPDATE host SET tenant=? WHERE id in ("+str.IdsString(ids)+")", tenant)
	return err
}

func HostGet(where string, args ...interface{}) (*Host, error) {
	var obj Host
	has, err := DB["ams"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func HostGets(where string, args ...interface{}) (hosts []Host, err error) {
	if where != "" {
		err = DB["ams"].Where(where, args...).Find(&hosts)
	} else {
		err = DB["ams"].Find(&hosts)
	}
	return hosts, err
}

func HostByIds(ids []int64) (hosts []Host, err error) {
	if len(ids) == 0 {
		return
	}

	err = DB["ams"].In("id", ids).Find(&hosts)
	return
}

func HostIdsByIps(ips []string) (ids []int64, err error) {
	err = DB["ams"].Table(new(Host)).In("ip", ips).Select("id").Find(&ids)
	return
}

func HostSearch(batch, field string) ([]Host, error) {
	arr := str.ParseLines(strings.Replace(batch, ",", "\n", -1))
	if len(arr) == 0 {
		return []Host{}, nil
	}

	var objs []Host
	err := DB["ams"].In(field, arr).Find(&objs)
	return objs, err
}

func HostTotalForAdmin(tenant, query, batch, field string) (int64, error) {
	return buildHostWhere(tenant, query, batch, field).Count()
}

func HostGetsForAdmin(tenant, query, batch, field string, limit, offset int) ([]Host, error) {
	var objs []Host
	err := buildHostWhere(tenant, query, batch, field).Limit(limit, offset).Find(&objs)
	return objs, err
}

func buildHostWhere(tenant, query, batch, field string) *xorm.Session {
	session := DB["ams"].Table(new(Host)).OrderBy("ident")

	if tenant != "" {
		session = session.Where("tenant=?", tenant)
	}

	if batch == "" && query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("cate=? or sn=? or ident like ? or ip like ? or name like ? or note like ?", arr[i], arr[i], q, q, q, q)
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
