package models

import (
	"errors"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type BoardPayload struct {
	Id      int64  `json:"id" gorm:"primaryKey"`
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
