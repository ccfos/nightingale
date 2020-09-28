package models

type Chart struct {
	Id         int64  `json:"id"`
	SubclassId int64  `json:"subclass_id"`
	Configs    string `json:"configs"`
	Weight     int    `json:"weight"`
}

func (c *Chart) Add() error {
	_, err := DB["mon"].InsertOne(c)
	return err
}

func ChartGets(subclassId int64) ([]Chart, error) {
	var objs []Chart
	err := DB["mon"].Where("subclass_id=?", subclassId).OrderBy("weight").Find(&objs)
	return objs, err
}

func ChartGet(col string, val interface{}) (*Chart, error) {
	var obj Chart
	has, err := DB["mon"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (c *Chart) Update(cols ...string) error {
	_, err := DB["mon"].Where("id=?", c.Id).Cols(cols...).Update(c)
	return err
}

func (c *Chart) Del() error {
	_, err := DB["mon"].Where("id=?", c.Id).Delete(new(Chart))
	return err
}
