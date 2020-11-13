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

func ContainerResourceSync(ids []int64, newItems []map[string]interface{}) error {
	if len(ids) == 0 {
		return nil
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	_, err := session.In("res_id", ids).Delete(new(NodeResource))
	if err != nil {
		session.Rollback()
		return err
	}

	_, err = session.In("id", ids).Delete(new(Resource))
	if err != nil {
		session.Rollback()
		return err
	}

	count := len(newItems)
	if count == 0 {
		return session.Commit()
	}

	for i := 0; i < count; i++ {
		id := newItems[i]["nid"].(int64)

		var node Node
		has, err := session.Where("id = ?", id).Get(&node)
		if err != nil {
			session.Rollback()
			return err
		}

		if !has {
			session.Rollback()
			return fmt.Errorf("no such node[id:%d]", id)
		}

		if node.Leaf != 1 {
			session.Rollback()
			return fmt.Errorf("node not leaf")
		}

		var res Resource
		has, err = session.Where("uuid = ?", newItems[i]["uuid"].(string)).Get(&res)
		if err != nil {
			session.Rollback()
			return err
		}

		if !has {
			// 之前没有过这个资源，在RDB注册这个资源
			res.UUID = newItems[i]["uuid"].(string)
			res.Ident = newItems[i]["ident"].(string)
			res.Name = newItems[i]["name"].(string)
			res.Labels = newItems[i]["labels"].(string)
			res.Extend = newItems[i]["extend"].(string)
			res.Cate = newItems[i]["cate"].(string)
			res.Tenant = node.Tenant()
			_, err := session.InsertOne(&res)
			if err != nil {
				session.Rollback()
				return err
			}
		} else {
			// 这个资源之前就已经存在过了，这次可能是更新了部分字段
			res.Name = newItems[i]["name"].(string)
			res.Labels = newItems[i]["labels"].(string)
			res.Extend = newItems[i]["extend"].(string)
			_, err := session.Where("id=?", res.Id).Cols("name", "labels", "extend").Update(&res)
			if err != nil {
				session.Rollback()
				return err
			}
		}

		tenant := node.Tenant()

		if tenant != InnerTenantIdent {
			// 所有机器必须属于这个租户才可以挂载到这个租户下的节点，唯独inner节点特殊，inner节点可以挂载其他租户的资源
			var notMine []Resource
			err := session.In("id", res.Id).Where("tenant<>?", tenant).Find(&notMine)
			if err != nil {
				session.Rollback()
				return err
			}

			size := len(notMine)
			if size > 0 {
				arr := make([]string, size)
				for i := 0; i < size; i++ {
					arr[i] = fmt.Sprintf("%s[%s]", notMine[i].Ident, notMine[i].Name)
				}
				session.Rollback()
				return fmt.Errorf("%s dont belong to tenant[%s]", strings.Join(arr, ", "), tenant)
			}
		}

		// 判断是否已经存在绑定关系
		total, err := session.Where("node_id=? and res_id=?",
			node.Id, res.Id).Count(new(NodeResource))
		if err != nil {
			session.Rollback()
			return err
		}

		if total <= 0 {
			// 绑定节点和资源
			_, err = session.Insert(&NodeResource{
				NodeId: node.Id,
				ResId:  res.Id,
			})
			if err != nil {
				session.Rollback()
				return err
			}
		}

		// 第二个挂载位置：inner.${cate}
		innerCatePath := "inner." + node.Ident
		var (
			obj Node
		)

		has, err = session.Where("path = ?", innerCatePath).Get(&obj)
		if err != nil {
			session.Rollback()
			return err
		}

		if !has {
			var inner Node
			has, err = session.Where("path = ?", "inner").Get(&inner)
			if err != nil {
				session.Rollback()
				return err
			}

			if !has {
				session.Rollback()
				return fmt.Errorf("inner node not exists")
			}

			if inner.Leaf == 1 {
				session.Rollback()
				return fmt.Errorf("parent node is leaf, cannot create child")
			}

			if node.Cate == "tenant" {
				session.Rollback()
				return fmt.Errorf("tenant node should be root node only")
			}

			if node.Cate == "project" && (inner.Cate != "tenant" && inner.Cate != "organization") {
				session.Rollback()
				return fmt.Errorf("project node should be under tenant or organization")
			}

			if node.Ident == "" {
				session.Rollback()
				return fmt.Errorf("ident is blank")
			}

			if !str.IsMatch(node.Ident, "^[a-z0-9\\-\\_]+$") {
				session.Rollback()
				return fmt.Errorf("ident invalid")
			}

			if len(node.Ident) >= 32 {
				session.Rollback()
				return fmt.Errorf("ident length should be less than 32")
			}

			var nc NodeCate
			has, err := session.Where("ident = ?", node.Cate).Get(&nc)
			if err != nil {
				session.Rollback()
				return err
			}

			if !has {
				session.Rollback()
				return fmt.Errorf("node-category[%s] not found", node.Cate)
			}

			child := Node{
				Pid:       inner.Id,
				Ident:     node.Ident,
				Name:      node.Name,
				Path:      inner.Path + "." + node.Ident,
				Leaf:      1,
				Cate:      node.Cate,
				IconColor: nc.IconColor,
				Proxy:     1,
				Note:      "",
				Creator:   "system",
			}

			child.IconChar = strings.ToUpper(node.Cate[0:1])

			if _, err = session.Insert(&child); err != nil {
				session.Rollback()
				return err
			}

			obj = child
		}

		if obj.Leaf != 1 {
			session.Rollback()
			return fmt.Errorf("node[%s] not leaf", obj.Path)
		}

		total, err = session.Where("node_id=? and res_id=?", obj.Id, res.Id).Count(new(NodeResource))
		if err != nil {
			session.Rollback()
			return err
		}

		if total <= 0 {
			// 绑定节点和资源
			_, err = session.Insert(&NodeResource{
				NodeId: obj.Id,
				ResId:  res.Id,
			})
			if err != nil {
				session.Rollback()
				return err
			}
		}
	}

	return session.Commit()
}
