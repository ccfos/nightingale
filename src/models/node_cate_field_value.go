package models

import (
	"fmt"
)

type NodeFieldValue struct {
	Id         int64  `json:"id"`
	NodeId     int64  `json:"node_id"`
	FieldIdent string `json:"field_ident"`
	FieldValue string `json:"field_value"`
}

func (nfv *NodeFieldValue) Validate() error {
	if len(nfv.FieldValue) > 255 {
		return fmt.Errorf("field value too long")
	}
	return nil
}

// NodeFieldValueGets 条数非常少，全部返回
func NodeFieldValueGets(nodeId int64) ([]NodeFieldValue, error) {
	var objs []NodeFieldValue
	err := DB["rdb"].Where("node_id = ?", nodeId).OrderBy("field_ident").Find(&objs)
	return objs, err
}

func NodeFieldValuePuts(nodeId int64, objs []NodeFieldValue) error {
	count := len(objs)

	session := DB["rdb"].NewSession()
	defer session.Close()

	for i := 0; i < count; i++ {
		num, err := session.Where("node_id = ? and field_ident = ?", nodeId, objs[i].FieldIdent).Count(new(NodeFieldValue))
		if err != nil {
			return fmt.Errorf("count node_field_value fail: %v", err)
		}

		if num > 0 {
			_, err = session.Exec("UPDATE node_field_value SET field_value = ? WHERE node_id = ? and field_ident = ?", objs[i].FieldValue, nodeId, objs[i].FieldIdent)
			if err != nil {
				return fmt.Errorf("update node_field_value fail: %v", err)
			}
		} else {
			_, err = session.InsertOne(NodeFieldValue{
				NodeId:     nodeId,
				FieldIdent: objs[i].FieldIdent,
				FieldValue: objs[i].FieldValue,
			})
			if err != nil {
				return fmt.Errorf("insert node_field_value fail: %v", err)
			}
		}
	}

	return nil
}
