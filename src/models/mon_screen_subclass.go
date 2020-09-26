package models

type ScreenSubclass struct {
	Id       int64  `json:"id"`
	ScreenId int64  `json:"screen_id"`
	Name     string `json:"name"`
	Weight   int    `json:"weight"`
}

func (s *ScreenSubclass) Add() error {
	_, err := DB["mon"].Insert(s)
	return err
}

func ScreenSubclassGets(screenId int64) ([]ScreenSubclass, error) {
	var objs []ScreenSubclass
	err := DB["mon"].Where("screen_id=?", screenId).OrderBy("weight").Find(&objs)
	return objs, err
}

func ScreenSubclassGet(col string, val interface{}) (*ScreenSubclass, error) {
	var obj ScreenSubclass
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (s *ScreenSubclass) Update(cols ...string) error {
	_, err := DB["mon"].Where("id=?", s.Id).Cols(cols...).Update(s)
	return err
}

func (s *ScreenSubclass) Del() error {
	_, err := DB["mon"].Where("subclass_id=?", s.Id).Delete(new(Chart))
	if err != nil {
		return err
	}

	_, err = DB["mon"].Where("id=?", s.Id).Delete(new(ScreenSubclass))
	return err
}
