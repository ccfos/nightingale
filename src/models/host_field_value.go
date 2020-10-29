package models

import (
	"fmt"
)

type HostFieldValue struct {
	Id         int64  `json:"id"`
	HostId     int64  `json:"host_id"`
	FieldIdent string `json:"field_ident"`
	FieldValue string `json:"field_value"`
}

func (hfv *HostFieldValue) Validate() error {
	if len(hfv.FieldValue) > 1024 {
		return fmt.Errorf("field value too long")
	}
	return nil
}

// HostFieldValueGets 条数非常少，全部返回
func HostFieldValueGets(hostId int64) ([]HostFieldValue, error) {
	var objs []HostFieldValue
	err := DB["ams"].Where("host_id = ?", hostId).OrderBy("field_ident").Find(&objs)
	return objs, err
}

func HostFieldValuePuts(hostId int64, objs []HostFieldValue) error {
	count := len(objs)

	session := DB["ams"].NewSession()
	defer session.Close()

	for i := 0; i < count; i++ {
		num, err := session.Where("host_id = ? and field_ident = ?", hostId, objs[i].FieldIdent).Count(new(HostFieldValue))
		if err != nil {
			return fmt.Errorf("count host_field_value fail: %v", err)
		}

		if num > 0 {
			_, err = session.Exec("UPDATE host_field_value SET field_value = ? WHERE host_id = ? and field_ident = ?", objs[i].FieldValue, hostId, objs[i].FieldIdent)
			if err != nil {
				return fmt.Errorf("update host_field_value fail: %v", err)
			}
		} else {
			_, err = session.InsertOne(HostFieldValue{
				HostId:     hostId,
				FieldIdent: objs[i].FieldIdent,
				FieldValue: objs[i].FieldValue,
			})
			if err != nil {
				return fmt.Errorf("insert host_field_value fail: %v", err)
			}
		}
	}

	return nil
}
