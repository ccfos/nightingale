package models

import (
	"fmt"
)

type HostField struct {
	Id            int64  `json:"id"`
	FieldIdent    string `json:"field_ident"`
	FieldName     string `json:"field_name"`
	FieldType     string `json:"field_type"`
	FieldRequired int    `json:"field_required"`
	FieldExtra    string `json:"field_extra"`
	FieldCate     string `json:"field_cate"`
}

func (hf *HostField) Validate() error {
	if len(hf.FieldIdent) > 255 {
		return fmt.Errorf("field ident too long")
	}

	if len(hf.FieldName) > 255 {
		return fmt.Errorf("field name too long")
	}

	if len(hf.FieldExtra) > 2048 {
		return fmt.Errorf("field extra too long")
	}

	if len(hf.FieldCate) > 255 {
		return fmt.Errorf("field cate too long")
	}

	return nil
}

func HostFieldNew(objPtr *HostField) error {
	if err := objPtr.Validate(); err != nil {
		return err
	}

	cnt, err := DB["ams"].Where("field_ident=?", objPtr.FieldIdent).Count(new(HostField))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("%s already exists", objPtr.FieldIdent)
	}

	_, err = DB["ams"].Insert(objPtr)
	return err
}

func (hf *HostField) Update(cols ...string) error {
	if err := hf.Validate(); err != nil {
		return err
	}

	_, err := DB["ams"].Where("id=?", hf.Id).Cols(cols...).Update(hf)
	return err
}

func HostFieldGet(where string, args ...interface{}) (*HostField, error) {
	var obj HostField
	has, err := DB["ams"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (hf *HostField) Del() error {
	_, err := DB["ams"].Where("id=?", hf.Id).Delete(new(HostField))
	return err
}

// HostFieldGets 条数非常少，全部返回
func HostFieldGets() ([]HostField, error) {
	var objs []HostField
	err := DB["ams"].OrderBy("field_cate, field_ident").Find(&objs)
	return objs, err
}
