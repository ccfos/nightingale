package model

import (
	"strings"

	"xorm.io/xorm"

	"github.com/toolkits/pkg/str"
)

type Endpoint struct {
	Id    int64  `json:"id"`
	Ident string `json:"ident"`
	Alias string `json:"alias"`
}

func EndpointGet(col string, val interface{}) (*Endpoint, error) {
	var obj Endpoint
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (e *Endpoint) Update(cols ...string) error {
	_, err := DB["mon"].Where("id=?", e.Id).Cols(cols...).Update(e)
	return err
}

func EndpointTotal(query, batch, field string) (int64, error) {
	session := buildEndpointWhere(query, batch, field)
	return session.Count(new(Endpoint))
}

func EndpointGets(query, batch, field string, limit, offset int) ([]Endpoint, error) {
	session := buildEndpointWhere(query, batch, field).OrderBy(field).Limit(limit, offset)
	var objs []Endpoint
	err := session.Find(&objs)
	return objs, err
}

func buildEndpointWhere(query, batch, field string) *xorm.Session {
	session := DB["mon"].Table(new(Endpoint))

	if batch == "" && query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("ident like ? or alias like ?", q, q)
		}
	}

	if batch != "" {
		endpoints := str.ParseCommaTrim(batch)
		if len(endpoints) > 0 {
			session = session.In(field, endpoints)
		}
	}

	return session
}

func EndpointImport(endpoints []string) error {
	count := len(endpoints)
	if count == 0 {
		return nil
	}

	session := DB["mon"].NewSession()
	defer session.Close()

	for i := 0; i < count; i++ {
		arr := strings.Split(endpoints[i], "::")

		ident := strings.TrimSpace(arr[0])
		alias := ""
		if len(arr) == 2 {
			alias = strings.TrimSpace(arr[1])
		}

		if ident == "" {
			continue
		}

		err := endpointImport(session, ident, alias)
		if err != nil {
			return err
		}
	}

	return nil
}

func endpointImport(session *xorm.Session, ident, alias string) error {
	var endpoint Endpoint
	has, err := session.Where("ident=?", ident).Get(&endpoint)
	if err != nil {
		return err
	}

	if has {
		if alias != "" {
			endpoint.Alias = alias
			_, err = session.Where("ident=?", ident).Cols("alias").Update(endpoint)
		}
	} else {
		_, err = session.Insert(Endpoint{Ident: ident, Alias: alias})
	}

	return err
}

func EndpointDel(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	bindings, err := NodeEndpointGetByEndpointIds(ids)
	if err != nil {
		return err
	}

	for i := 0; i < len(bindings); i++ {
		err = NodeEndpointUnbind(bindings[i].NodeId, bindings[i].EndpointId)
		if err != nil {
			return err
		}
	}

	if _, err := DB["mon"].In("id", ids).Delete(new(Endpoint)); err != nil {
		return err
	}

	return nil
}

func buildEndpointUnderNodeWhere(leafids []int64, query, batch, field string) *xorm.Session {
	session := DB["mon"].Where("id in (select endpoint_id from node_endpoint where node_id in (" + str.IdsString(leafids) + "))")

	if batch == "" && query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("ident like ? or alias like ?", q, q)
		}
	}

	if batch != "" {
		endpoints := str.ParseCommaTrim(batch)
		if len(endpoints) > 0 {
			session = session.In(field, endpoints)
		}
	}

	return session
}

func EndpointUnderNodeTotal(leafids []int64, query, batch, field string) (int64, error) {
	session := buildEndpointUnderNodeWhere(leafids, query, batch, field)
	return session.Count(new(Endpoint))
}

func EndpointUnderNodeGets(leafids []int64, query, batch, field string, limit, offset int) ([]Endpoint, error) {
	session := buildEndpointUnderNodeWhere(leafids, query, batch, field).Limit(limit, offset).OrderBy(field)
	var objs []Endpoint
	err := session.Find(&objs)
	return objs, err
}

func EndpointIdsByIdents(idents []string) ([]int64, error) {
	idents = str.TrimStringSlice(idents)
	if len(idents) == 0 {
		return []int64{}, nil
	}

	var objs []Endpoint
	err := DB["mon"].In("ident", idents).Find(&objs)
	if err != nil {
		return []int64{}, err
	}

	cnt := len(objs)
	ret := make([]int64, 0, cnt)
	for i := 0; i < cnt; i++ {
		ret = append(ret, objs[i].Id)
	}

	return ret, nil
}

type EndpointBinding struct {
	Ident string `json:"ident"`
	Alias string `json:"alias"`
	Nodes []Node `json:"nodes"`
}

func EndpointBindings(endpointIds []int64) ([]EndpointBinding, error) {
	var nes []NodeEndpoint
	err := DB["mon"].In("endpoint_id", endpointIds).Find(&nes)
	if err != nil {
		return []EndpointBinding{}, err
	}

	cnt := len(nes)
	if cnt == 0 {
		return []EndpointBinding{}, nil
	}

	h2n := make(map[int64][]int64)
	arr := make([]int64, 0, cnt)
	for i := 0; i < cnt; i++ {
		arr = append(arr, nes[i].EndpointId)
		h2n[nes[i].EndpointId] = append(h2n[nes[i].EndpointId], nes[i].NodeId)
	}

	var endpoints []Endpoint
	err = DB["mon"].In("id", arr).Find(&endpoints)
	if err != nil {
		return []EndpointBinding{}, err
	}

	cnt = len(endpoints)
	ret := make([]EndpointBinding, 0, cnt)
	for i := 0; i < cnt; i++ {
		nodeids := h2n[endpoints[i].Id]
		if len(nodeids) == 0 {
			continue
		}

		var nodes []Node
		err = DB["mon"].In("id", nodeids).Find(&nodes)
		if err != nil {
			return []EndpointBinding{}, err
		}

		b := EndpointBinding{
			Ident: endpoints[i].Ident,
			Alias: endpoints[i].Alias,
			Nodes: nodes,
		}

		ret = append(ret, b)
	}

	return ret, nil
}

func EndpointUnderLeafs(leafIds []int64) ([]Endpoint, error) {
	var endpoints []Endpoint
	if len(leafIds) == 0 {
		return []Endpoint{}, nil
	}

	err := DB["mon"].Where("id in (select endpoint_id from node_endpoint where node_id in (" + str.IdsString(leafIds) + "))").Find(&endpoints)
	return endpoints, err
}
