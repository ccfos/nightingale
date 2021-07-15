package models

import (
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
)

type Dashboard struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Tags     string `json:"tags"`
	Configs  string `json:"configs"`
	Favorite int    `json:"favorite" xorm:"-"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

func (d *Dashboard) TableName() string {
	return "dashboard"
}

func (d *Dashboard) Validate() error {
	if d.Name == "" {
		return _e("Dashboard name is empty")
	}

	if str.Dangerous(d.Name) {
		return _e("Dashboard name has invalid characters")
	}
	return nil
}

func (d *Dashboard) FillFavorite(ids []int64) {
	if slice.ContainsInt64(ids, d.Id) {
		d.Favorite = 1
	}
}

func DashboardCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(Dashboard))
	if err != nil {
		logger.Errorf("mysql.error: count dashboard fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func (d *Dashboard) AddOnly() error {
	if err := d.Validate(); err != nil {
		return err
	}

	num, err := DashboardCount("name=?", d.Name)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("Dashboard %s already exists", d.Name)
	}

	now := time.Now().Unix()
	d.CreateAt = now
	d.UpdateAt = now
	err = DBInsertOne(d)
	return err
}

func (d *Dashboard) Add() error {
	if err := d.Validate(); err != nil {
		return err
	}

	num, err := DashboardCount("name=?", d.Name)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("Dashboard %s already exists", d.Name)
	}

	now := time.Now().Unix()
	d.CreateAt = now
	d.UpdateAt = now
	err = DBInsertOne(d)
	if err == nil {
		// 如果成功创建dashboard，可以自动创建一个default chart group，便于用户使用
		cg := ChartGroup{
			DashboardId: d.Id,
			Name:        "Default chart group",
			Weight:      0,
		}
		cg.Add()
	}

	return err
}

func (d *Dashboard) Update(cols ...string) error {
	if err := d.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", d.Id).Cols(cols...).Update(d)
	if err != nil {
		logger.Errorf("mysql.error: update dashboard(id=%d) fail: %v", d.Id, err)
		return internalServerError
	}

	return nil
}

func DashboardTotal(onlyfavorite bool, ids []int64, query string) (num int64, err error) {
	session := DB.NewSession()
	defer session.Close()

	if onlyfavorite {
		session = session.In("id", ids)
	}

	arr := strings.Fields(query)
	if len(arr) > 0 {
		for i := 0; i < len(arr); i++ {
			if strings.HasPrefix(arr[i], "-") {
				q := "%" + arr[i][1:] + "%"
				session = session.Where("name not like ? and tags not like ?", q, q)
			} else {
				q := "%" + arr[i] + "%"
				session = session.Where("(name like ? or tags like ?)", q, q)
			}
		}
	}

	num, err = session.Count(new(Dashboard))
	if err != nil {
		logger.Errorf("mysql.error: count dashboard fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func DashboardGets(onlyfavorite bool, ids []int64, query string, limit, offset int) ([]Dashboard, error) {
	session := DB.Limit(limit, offset).OrderBy("name")

	if onlyfavorite {
		session = session.In("id", ids)
	}

	arr := strings.Fields(query)
	if len(arr) > 0 {
		for i := 0; i < len(arr); i++ {
			if strings.HasPrefix(arr[i], "-") {
				q := "%" + arr[i][1:] + "%"
				session = session.Where("name not like ? and tags not like ?", q, q)
			} else {
				q := "%" + arr[i] + "%"
				session = session.Where("(name like ? or tags like ?)", q, q)
			}
		}
	}

	// configs字段内容太多，列表页面不需要
	var objs []Dashboard
	err := session.Cols("id", "name", "tags", "create_at", "create_by", "update_at", "update_by").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query dashboard fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []Dashboard{}, nil
	}

	return objs, nil
}

func DashboardGetsByIds(ids []int64) ([]*Dashboard, error) {
	var objs []*Dashboard
	err := DB.In("id", ids).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query dashboards(%v) fail: %v", ids, err)
		return nil, internalServerError
	}

	return objs, nil
}

func DashboardGet(where string, args ...interface{}) (*Dashboard, error) {
	var obj Dashboard
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query dashboard(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (d *Dashboard) Del() error {
	session := DB.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM chart WHERE group_id in (select id from chart_group where dashboard_id=?)", d.Id); err != nil {
		logger.Errorf("mysql.error: delete chart fail: %v", err)
		return err
	}

	if _, err := session.Exec("DELETE FROM chart_group WHERE dashboard_id=?", d.Id); err != nil {
		logger.Errorf("mysql.error: delete chart_group fail: %v", err)
		return err
	}

	if _, err := session.Exec("DELETE FROM dashboard WHERE id=?", d.Id); err != nil {
		logger.Errorf("mysql.error: delete dashboard fail: %v", err)
		return err
	}

	return session.Commit()
}
