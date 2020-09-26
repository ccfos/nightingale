package models

import (
	"time"
)

type Screen struct {
	Id          int64     `json:"id"`
	NodeId      int64     `json:"node_id"`
	Name        string    `json:"name"`
	LastUpdator string    `json:"last_updator"`
	LastUpdated time.Time `xorm:"<-" json:"last_updated"`

	NodePath string `json:"node_path" xorm:"-"`
}

func (s *Screen) Add() error {
	_, err := DB["mon"].Insert(s)
	return err
}

func ScreenGets(nodeId int64) ([]Screen, error) {
	var objs []Screen
	err := DB["mon"].Where("node_id=?", nodeId).OrderBy("name").Find(&objs)
	return objs, err
}

func ScreenGet(col string, val interface{}) (*Screen, error) {
	var obj Screen
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (s *Screen) Update(cols ...string) error {
	_, err := DB["mon"].Where("id=?", s.Id).Cols(cols...).Update(s)
	return err
}

func (s *Screen) Del() error {
	subclasses, err := ScreenSubclassGets(s.Id)
	if err != nil {
		return err
	}

	cnt := len(subclasses)
	for i := 0; i < cnt; i++ {
		err = subclasses[i].Del()
		if err != nil {
			return err
		}
	}

	_, err = DB["mon"].Where("id=? and node_id=?", s.Id, s.NodeId).Delete(new(Screen))
	return err
}
