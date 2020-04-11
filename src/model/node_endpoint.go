package model

import (
	"fmt"
)

type NodeEndpoint struct {
	NodeId     int64 `xorm:"'node_id'"`
	EndpointId int64 `xorm:"'endpoint_id'"`
}

func (NodeEndpoint) TableName() string {
	return "node_endpoint"
}

func NodeIdsGetByEndpointId(endpointId int64) ([]int64, error) {
	if endpointId == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["mon"].Table("node_endpoint").Where("endpoint_id = ?", endpointId).Select("node_id").Find(&ids)
	return ids, err
}

func EndpointIdsByNodeIds(nodeIds []int64) ([]int64, error) {
	if len(nodeIds) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["mon"].Table("node_endpoint").In("node_id", nodeIds).Select("endpoint_id").Find(&ids)
	return ids, err
}

func NodeEndpointGetByEndpointIds(endpointsIds []int64) ([]NodeEndpoint, error) {
	if len(endpointsIds) == 0 {
		return []NodeEndpoint{}, nil
	}

	var objs []NodeEndpoint
	err := DB["mon"].In("endpoint_id", endpointsIds).Find(&objs)
	return objs, err
}

// EndpointBindingsForMail 用来发告警邮件的时候带上各个endpoint的挂载信息
func EndpointBindingsForMail(endpoints []string) []string {
	ids, err := EndpointIdsByIdents(endpoints)
	if err != nil {
		return []string{fmt.Sprintf("get endpoint ids by idents fail: %v", err)}
	}

	if len(ids) == 0 {
		return []string{}
	}

	bindings, err := EndpointBindings(ids)
	if err != nil {
		return []string{fmt.Sprintf("get endpoint bindings fail: %v", err)}
	}

	var ret []string
	size := len(bindings)
	for i := 0; i < size; i++ {
		for j := 0; j < len(bindings[i].Nodes); j++ {
			ret = append(ret, bindings[i].Ident+" - "+bindings[i].Alias+" - "+bindings[i].Nodes[j].Path)
		}
	}

	return ret
}

func NodeEndpointGetByNodeIds(nodeIds []int64) ([]NodeEndpoint, error) {
	if len(nodeIds) == 0 {
		return []NodeEndpoint{}, nil
	}

	var objs []NodeEndpoint
	err := DB["mon"].In("node_id", nodeIds).Find(&objs)
	return objs, err
}

func NodeEndpointUnbind(nid, eid int64) error {
	_, err := DB["mon"].Where("node_id=? and endpoint_id=?", nid, eid).Delete(new(NodeEndpoint))
	return err
}

func NodeEndpointBind(nid, eid int64) error {
	total, err := DB["mon"].Where("node_id=? and endpoint_id=?", nid, eid).Count(new(NodeEndpoint))
	if err != nil {
		return err
	}

	if total > 0 {
		return nil
	}

	endpoint, err := EndpointGet("id", eid)
	if err != nil {
		return err
	}

	if endpoint == nil {
		return fmt.Errorf("endpoint[id:%d] not found", eid)
	}

	_, err = DB["mon"].Insert(&NodeEndpoint{
		NodeId:     nid,
		EndpointId: eid,
	})

	return err
}
