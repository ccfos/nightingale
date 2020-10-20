package models

import (
	"fmt"
	"time"
)

type NodeCateField struct {
	Id            int64     `json:"id"`
	Cate          string    `json:"cate"`
	FieldIdent    string    `json:"field_ident"`
	FieldName     string    `json:"field_name"`
	FieldType     string    `json:"field_type"`
	FieldRequired int       `json:"field_required"`
	FieldExtra    string    `json:"field_extra"`
	LastUpdated   time.Time `json:"last_updated" xorm:"<-"`
}

func (ncf *NodeCateField) Validate() error {
	if len(ncf.FieldIdent) > 255 {
		return fmt.Errorf("field ident too long")
	}

	if len(ncf.FieldName) > 255 {
		return fmt.Errorf("field name too long")
	}

	if len(ncf.FieldExtra) > 2048 {
		return fmt.Errorf("field extra too long")
	}

	return nil
}

func NodeCateFieldNew(objPtr *NodeCateField) error {
	if err := objPtr.Validate(); err != nil {
		return err
	}

	cnt, err := DB["rdb"].Where("cate=? and field_ident=?", objPtr.Cate, objPtr.FieldIdent).Count(new(NodeCateField))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("%s already exists", objPtr.FieldIdent)
	}

	_, err = DB["rdb"].Insert(objPtr)
	return err
}

func (ncf *NodeCateField) Update(cols ...string) error {
	if err := ncf.Validate(); err != nil {
		return err
	}

	_, err := DB["rdb"].Where("id=?", ncf.Id).Cols(cols...).Update(ncf)
	return err
}

func NodeCateFieldGet(where string, args ...interface{}) (*NodeCateField, error) {
	var obj NodeCateField
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (ncf *NodeCateField) Del() error {
	_, err := DB["rdb"].Where("id=?", ncf.Id).Delete(new(NodeCateField))
	return err
}

// NodeCateFieldGets 条数非常少，全部返回
func NodeCateFieldGets(where string, args ...interface{}) ([]NodeCateField, error) {
	var objs []NodeCateField
	err := DB["rdb"].Where(where, args...).OrderBy("field_ident").Find(&objs)
	return objs, err
}
