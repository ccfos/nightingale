package models

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
)

type EsIndexPatternFe struct {
	Id                     int64  `json:"id" gorm:"primaryKey"`
	DatasourceId           int64  `json:"datasource_id"`
	Name                   string `json:"name"`
	TimeField              string `json:"time_field"`
	AllowHideSystemIndices bool   `json:"allow_hide_system_indices"`
	FieldsFormat           string `json:"fields_format"`
	CreateAt               int64  `json:"create_at"`
	CreateBy               string `json:"create_by"`
	UpdateAt               int64  `json:"update_at"`
	UpdateBy               string `json:"update_by"`
}

type EsIndexPattern struct {
	Id                     int64  `json:"id" gorm:"primaryKey"`
	DatasourceId           int64  `json:"datasource_id"`
	Name                   string `json:"name"`
	TimeField              string `json:"time_field"`
	AllowHideSystemIndices int    `json:"allow_hide_system_indices"`
	FieldsFormat           string `json:"fields_format"`
	CreateAt               int64  `json:"create_at"`
	CreateBy               string `json:"create_by"`
	UpdateAt               int64  `json:"update_at"`
	UpdateBy               string `json:"update_by"`
}

func (t *EsIndexPattern) TableName() string {
	return "es_index_pattern"
}

func (r *EsIndexPatternFe) Add(ctx *ctx.Context) error {
	esIndexPattern, err := EsIndexPatternGet(ctx, "datasource_id = ? and name = ?", r.DatasourceId, r.Name)
	if err != nil {
		return errors.WithMessage(err, "failed to query es index pattern")
	}

	if esIndexPattern != nil {
		return errors.New("es index pattern datasource and name already exists")
	}

	dbIndexPattern := r.FE2DB()

	return DB(ctx).Create(dbIndexPattern).Error
}

func EsIndexPatternDel(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(EsIndexPattern)).Error
}

func (ei *EsIndexPatternFe) Update(ctx *ctx.Context, eip EsIndexPatternFe) error {
	if ei.Name != eip.Name || ei.DatasourceId != eip.DatasourceId {
		exists, err := EsIndexPatternExists(ctx, ei.Id, eip.DatasourceId, eip.Name)
		if err != nil {
			return err
		}

		if exists {
			return errors.New("EsIndexPattern already exists")
		}
	}

	eip.Id = ei.Id
	eip.CreateAt = ei.CreateAt
	eip.CreateBy = ei.CreateBy
	eip.UpdateAt = time.Now().Unix()

	dbIndexPattern := eip.FE2DB()

	return DB(ctx).Model(dbIndexPattern).Select("*").Updates(dbIndexPattern).Error
}

func (dbIndexPatten *EsIndexPattern) DB2FE() *EsIndexPatternFe {
	feIndexPattern := new(EsIndexPatternFe)
	feIndexPattern.Id = dbIndexPatten.Id
	feIndexPattern.DatasourceId = dbIndexPatten.DatasourceId
	feIndexPattern.Name = dbIndexPatten.Name
	feIndexPattern.TimeField = dbIndexPatten.TimeField

	allowHideSystemIndices := false
	if dbIndexPatten.AllowHideSystemIndices != 0 {
		allowHideSystemIndices = true
	}

	feIndexPattern.AllowHideSystemIndices = allowHideSystemIndices
	feIndexPattern.FieldsFormat = dbIndexPatten.FieldsFormat
	feIndexPattern.CreateAt = dbIndexPatten.CreateAt
	feIndexPattern.UpdateAt = dbIndexPatten.UpdateAt
	feIndexPattern.UpdateBy = dbIndexPatten.UpdateBy

	return feIndexPattern
}

func (feIndexPatten *EsIndexPatternFe) FE2DB() *EsIndexPattern {
	dbIndexPattern := new(EsIndexPattern)
	dbIndexPattern.Id = feIndexPatten.Id
	dbIndexPattern.DatasourceId = feIndexPatten.DatasourceId
	dbIndexPattern.Name = feIndexPatten.Name
	dbIndexPattern.TimeField = feIndexPatten.TimeField

	allowHideSystemIndices := 0
	if !feIndexPatten.AllowHideSystemIndices {
		allowHideSystemIndices = 1
	}

	dbIndexPattern.AllowHideSystemIndices = allowHideSystemIndices
	dbIndexPattern.FieldsFormat = feIndexPatten.FieldsFormat
	dbIndexPattern.CreateAt = feIndexPatten.CreateAt
	dbIndexPattern.UpdateAt = feIndexPatten.UpdateAt
	dbIndexPattern.UpdateBy = feIndexPatten.UpdateBy

	return dbIndexPattern
}

func EsIndexPatternGets(ctx *ctx.Context, where string, args ...interface{}) ([]*EsIndexPatternFe, error) {
	var objs []*EsIndexPattern
	err := DB(ctx).Where(where, args...).Find(&objs).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to query es index pattern")
	}

	var finals []*EsIndexPatternFe
	for _, i := range objs {
		dbIndexPattern := i.DB2FE()
		finals = append(finals, dbIndexPattern)
	}
	return finals, nil
}

func EsIndexPatternGet(ctx *ctx.Context, where string, args ...interface{}) (*EsIndexPatternFe, error) {
	var lst []*EsIndexPattern
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	return lst[0].DB2FE(), nil
}

func EsIndexPatternGetById(ctx *ctx.Context, id int64) (*EsIndexPatternFe, error) {
	return EsIndexPatternGet(ctx, "id=?", id)
}

func EsIndexPatternExists(ctx *ctx.Context, id, datasourceId int64, name string) (bool, error) {
	session := DB(ctx).Where("id <> ? and datasource_id = ? and name = ?", id, datasourceId, name)

	var lst []EsIndexPattern
	err := session.Find(&lst).Error
	if err != nil {
		return false, err
	}
	if len(lst) == 0 {
		return false, nil
	}

	return true, nil
}
