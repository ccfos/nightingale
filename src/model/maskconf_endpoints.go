package model

type MaskconfEndpoints struct {
	Id       int64  `json:"id"`
	MaskId   int64  `json:"mask_id"`
	Endpoint string `json:"endpoint"`
}
