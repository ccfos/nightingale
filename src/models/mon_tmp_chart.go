package models

type TmpChart struct {
	Id      int64  `json:"id"`
	Configs string `json:"configs"`
	Creator string `json:"creator"`
}

func (t *TmpChart) Add() error {
	_, err := DB["mon"].InsertOne(t)
	return err
}

func TmpChartGet(col string, val interface{}) (*TmpChart, error) {
	var obj TmpChart
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}
