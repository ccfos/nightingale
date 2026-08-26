package models

import (
	"errors"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type BoardPayload struct {
	// id 就是 board.id，由调用方赋值，不是自增列（见 docker/initsql/a-n9e.sql）。
	// 不写 autoIncrement:false 的话 gorm 会按自增处理，达梦驱动插入前会先发
	// SET IDENTITY_INSERT，而从 MySQL 迁移过来的表上没有自增列，直接报
	// -2717 表[board_payload]不存在IDENTITY列。
	Id      int64  `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Payload string `json:"payload"`
}

func (p *BoardPayload) TableName() string {
	return "board_payload"
}

func (p *BoardPayload) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	return DB(ctx).Model(p).Select(selectField, selectFields...).Updates(p).Error
}

func BoardPayloadGets(ctx *ctx.Context, ids []int64) ([]*BoardPayload, error) {
	if len(ids) == 0 {
		return nil, errors.New("empty ids")
	}

	var arr []*BoardPayload
	err := DB(ctx).Where("id in ?", ids).Find(&arr).Error
	return arr, err
}

func BoardPayloadGet(ctx *ctx.Context, id int64) (string, error) {
	payloads, err := BoardPayloadGets(ctx, []int64{id})
	if err != nil {
		return "", err
	}

	if len(payloads) == 0 {
		return "", nil
	}

	return payloads[0].Payload, nil
}

func BoardPayloadSave(ctx *ctx.Context, id int64, payload string) error {
	var bp BoardPayload
	err := DB(ctx).Where("id = ?", id).Find(&bp).Error
	if err != nil {
		return err
	}

	if bp.Id > 0 {
		// already exists
		bp.Payload = payload
		return bp.Update(ctx, "payload")
	}

	return Insert(ctx, &BoardPayload{
		Id:      id,
		Payload: payload,
	})
}
