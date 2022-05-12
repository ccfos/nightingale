package models

import "errors"

type BoardPayload struct {
	Id      int64  `json:"id" gorm:"primaryKey"`
	Payload string `json:"payload"`
}

func (p *BoardPayload) TableName() string {
	return "board_payload"
}

func (p *BoardPayload) Update(selectField interface{}, selectFields ...interface{}) error {
	return DB().Model(p).Select(selectField, selectFields...).Updates(p).Error
}

func BoardPayloadGets(ids []int64) ([]*BoardPayload, error) {
	if len(ids) == 0 {
		return nil, errors.New("empty ids")
	}

	var arr []*BoardPayload
	err := DB().Where("id in ?", ids).Find(&arr).Error
	return arr, err
}

func BoardPayloadGet(id int64) (string, error) {
	payloads, err := BoardPayloadGets([]int64{id})
	if err != nil {
		return "", err
	}

	if len(payloads) == 0 {
		return "", nil
	}

	return payloads[0].Payload, nil
}

func BoardPayloadSave(id int64, payload string) error {
	var bp BoardPayload
	err := DB().Where("id = ?", id).Find(&bp).Error
	if err != nil {
		return err
	}

	if bp.Id > 0 {
		// already exists
		bp.Payload = payload
		return bp.Update("payload")
	}

	return Insert(&BoardPayload{
		Id:      id,
		Payload: payload,
	})
}
