package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"xorm.io/xorm"

	"github.com/toolkits/pkg/str"
)

type Resource struct {
	Id          int64     `json:"id"`
	UUID        string    `json:"uuid" xorm:"'uuid'"`
	Ident       string    `json:"ident"`
	Name        string    `json:"name"`
	Labels      string    `json:"labels"`
	Note        string    `json:"note"`
	Extend      string    `json:"extend"`
	Cate        string    `json:"cate"`
	Tenant      string    `json:"tenant"`
	LastUpdated time.Time `json:"last_updated" xorm:"<-"`
}

func (r *Resource) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", r.Id).Cols(cols...).Update(r)
	return err
}

func (r *Resource) Save() error {
	_, err := DB["rdb"].InsertOne(r)
	return err
}

func ResourceIdsByUUIDs(uuids []string) ([]int64, error) {
	if len(uuids) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["rdb"].Table(new(Resource)).In("uuid", uuids).Select("id").Find(&ids)
	return ids, err
}

func ResourceIdsByIdents(idents []string) ([]int64, error) {
	if len(idents) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["rdb"].Table(new(Resource)).In("ident", idents).Select("id").Find(&ids)
	return ids, err
}

func ResourceIdentsByIds(ids []int64) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}

	var idents []string
	err := DB["rdb"].Table(new(Resource)).In("id", ids).Select("ident").Find(&idents)
	return idents, err
}

func ResourceGet(where string, args ...interface{}) (*Resource, error) {
	var obj Resource
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func ResourceGets(where string, args ...interface{}) ([]Resource, error) {
	var objs []Resource
	err := DB["rdb"].Where(where, args...).Find(&objs)
	return objs, err
}

func ResourceSearch(batch, field string) ([]Resource, error) {
	arr := str.ParseLines(strings.Replace(batch, ",", "\n", -1))
	if len(arr) == 0 {
		return []Resource{}, nil
	}

	var objs []Resource
	err := DB["rdb"].In(field, arr).Find(&objs)
	return objs, err
}

type ResourceBinding struct {
	Id    int64  `json:"id"`
	UUID  string `json:"uuid"`
	Ident string `json:"ident"`
	Name  string `json:"name"`
	Nodes []Node `json:"nodes"`
}

// ResourceBindings 资源与节点的绑定关系，一个资源对应多个节点
func ResourceBindings(resIds []int64) ([]ResourceBinding, error) {
	if len(resIds) == 0 {
		return []ResourceBinding{}, nil
	}

	var nrs []NodeResource
	err := DB["rdb"].In("res_id", resIds).Find(&nrs)
	if err != nil {
		return []ResourceBinding{}, err
	}

	cnt := len(nrs)
	if cnt == 0 {
		return []ResourceBinding{}, nil
	}

	r2n := make(map[int64][]int64)
	arr := make([]int64, 0, cnt)
	for i := 0; i < cnt; i++ {
		arr = append(arr, nrs[i].ResId)
		r2n[nrs[i].ResId] = append(r2n[nrs[i].ResId], nrs[i].NodeId)
	}

	var resources []Resource
	err = DB["rdb"].In("id", arr).Find(&resources)
	if err != nil {
		return []ResourceBinding{}, err
	}

	cnt = len(resources)
	ret := make([]ResourceBinding, 0, cnt)
	for i := 0; i < cnt; i++ {
		nodeIds := r2n[resources[i].Id]

		b := ResourceBinding{
			Id:    resources[i].Id,
			UUID:  resources[i].UUID,
			Ident: resources[i].Ident,
			Name:  resources[i].Name,
		}

		if nodeIds == nil || len(nodeIds) == 0 {
			b.Nodes = []Node{}
			ret = append(ret, b)
			continue
		}

		var nodes []Node
		err = DB["rdb"].In("id", nodeIds).Find(&nodes)
		if err != nil {
			return []ResourceBinding{}, err
		}

		b.Nodes = nodes
		ret = append(ret, b)
	}

	return ret, nil
}

// ResourceBindingsForMon 告警消息里要看到资源挂载的节点信息
func ResourceBindingsForMon(idents []string) ([]string, error) {
	ids, err := ResourceIdsByIdents(idents)
	if err != nil {
		return nil, err
	}

	bindings, err := ResourceBindings(ids)
	if err != nil {
		return nil, err
	}

	count := len(bindings)
	if count == 0 {
		return []string{}, nil
	}

	var ret []string
	for i := 0; i < count; i++ {
		for j := 0; j < len(bindings[i].Nodes); j++ {
			ret = append(ret, bindings[i].Ident+" - "+bindings[i].Name+" - "+bindings[i].Nodes[j].Path)
		}
	}

	return ret, nil
}

func buildResWhere(tenant, query, batch, field string) *xorm.Session {
	session := DB["rdb"].Table(new(Resource))

	if tenant != "" {
		session = session.Where("tenant=?", tenant)
	}

	if batch == "" && query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("cate = ? or uuid = ? or ident like ? or name like ? or note like ? or labels like ?", arr[i], arr[i], q, q, q, q)
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

func ResourceOrphanTotal(tenant, query, batch, field string) (int64, error) {
	session := buildResWhere(tenant, query, batch, field)
	return session.Where("id not in (select res_id from node_resource)").Count()
}

func ResourceOrphanList(tenant, query, batch, field string, limit, offset int) ([]Resource, error) {
	session := buildResWhere(tenant, query, batch, field)
	var objs []Resource
	err := session.Where("id not in (select res_id from node_resource)").OrderBy("ident").Limit(limit, offset).Find(&objs)
	return objs, err
}

func ResourceUnderNodeTotal(leafIds []int64, query, batch, field string) (int64, error) {
	session := buildResWhere("", query, batch, field)
	return session.Where("id in (select res_id from node_resource where node_id in (" + str.IdsString(leafIds) + "))").Count()
}

func ResourceUnderNodeGets(leafIds []int64, query, batch, field string, limit, offset int) ([]Resource, error) {
	session := buildResWhere("", query, batch, field)

	rids, err := ResIdsGetByNodeIds(leafIds)
	if err != nil {
		return nil, err
	}

	var objs []Resource
	if len(rids) == 0 {
		return objs, err
	}

	err = session.Where("id in ("+str.IdsString(rids)+")").OrderBy("ident").Limit(limit, offset).Find(&objs)
	return objs, err
}

func ResourceUnregister(uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}

	ids, err := ResourceIdsByUUIDs(uuids)
	if err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	err = NodeResourceUnbindByRids(ids)
	if err != nil {
		return err
	}

	_, err = DB["rdb"].In("id", ids).Delete(new(Resource))
	return err
}

// ResourceRegister 资源分配给某个租户的时候调用
func ResourceRegister(hosts []Host, tenant string) error {
	count := len(hosts)
	for i := 0; i < count; i++ {
		uuid := fmt.Sprintf("host-%d", hosts[i].Id)
		res, err := ResourceGet("uuid=?", uuid)
		if err != nil {
			return err
		}

		if res == nil {
			res = &Resource{
				UUID:   uuid,
				Ident:  hosts[i].Ident,
				Name:   hosts[i].Name,
				Cate:   hosts[i].Cate,
				Tenant: tenant,
			}

			// 如果host加个字段，并且要放到extend里，这里要改
			fields := map[string]interface{}{
				"cpu":  hosts[i].CPU,
				"mem":  hosts[i].Mem,
				"disk": hosts[i].Disk,
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
			if res.Tenant != "" && res.Tenant != tenant {
				// 之前有归属，如果归属发生变化，解除之前的挂载关系
				err = NodeResourceUnbindByRids([]int64{res.Id})
				if err != nil {
					return err
				}
			}
			res.Ident = hosts[i].Ident
			res.Name = hosts[i].Name
			res.Cate = hosts[i].Cate
			res.Tenant = tenant

			fields := map[string]interface{}{
				"cpu":  hosts[i].CPU,
				"mem":  hosts[i].Mem,
				"disk": hosts[i].Disk,
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
