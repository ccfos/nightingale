package models

import (
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

type Dashboard struct {
	Id       int64    `json:"id" gorm:"primaryKey"`
	GroupId  int64    `json:"group_id"`
	Name     string   `json:"name"`
	Tags     string   `json:"-"`
	TagsLst  []string `json:"tags" gorm:"-"`
	Configs  string   `json:"configs"`
	CreateAt int64    `json:"create_at"`
	CreateBy string   `json:"create_by"`
	UpdateAt int64    `json:"update_at"`
	UpdateBy string   `json:"update_by"`
}

func (d *Dashboard) TableName() string {
	return "dashboard"
}

func (d *Dashboard) Verify() error {
	if d.Name == "" {
		return errors.New("Name is blank")
	}

	if str.Dangerous(d.Name) {
		return errors.New("Name has invalid characters")
	}

	return nil
}

func (d *Dashboard) Add(ctx *ctx.Context) error {
	if err := d.Verify(); err != nil {
		return err
	}

	exists, err := DashboardExists(ctx, "group_id=? and name=?", d.GroupId, d.Name)
	if err != nil {
		return errors.WithMessage(err, "failed to count dashboard")
	}

	if exists {
		return errors.New("Dashboard already exists")
	}

	now := time.Now().Unix()
	d.CreateAt = now
	d.UpdateAt = now

	return Insert(ctx, d)
}

func (d *Dashboard) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if err := d.Verify(); err != nil {
		return err
	}

	return DB(ctx).Model(d).Select(selectField, selectFields...).Updates(d).Error
}

func (d *Dashboard) Del(ctx *ctx.Context) error {
	cgids, err := ChartGroupIdsOf(ctx, d.Id)
	if err != nil {
		return err
	}

	if len(cgids) == 0 {
		return DB(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("id=?", d.Id).Delete(&Dashboard{}).Error; err != nil {
				return err
			}
			return nil
		})
	}

	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id in ?", cgids).Delete(&Chart{}).Error; err != nil {
			return err
		}

		if err := tx.Where("dashboard_id=?", d.Id).Delete(&ChartGroup{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", d.Id).Delete(&Dashboard{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func DashboardGet(ctx *ctx.Context, where string, args ...interface{}) (*Dashboard, error) {
	var lst []*Dashboard
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].TagsLst = strings.Fields(lst[0].Tags)

	return lst[0], nil
}

func DashboardCount(ctx *ctx.Context, where string, args ...interface{}) (num int64, err error) {
	return Count(DB(ctx).Model(&Dashboard{}).Where(where, args...))
}

func DashboardExists(ctx *ctx.Context, where string, args ...interface{}) (bool, error) {
	num, err := DashboardCount(ctx, where, args...)
	return num > 0, err
}

func DashboardGets(ctx *ctx.Context, groupId int64, query string) ([]Dashboard, error) {
	session := DB(ctx).Where("group_id=?", groupId).Order("name")

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

	var objs []Dashboard
	err := session.Select("id", "group_id", "name", "tags", "create_at", "create_by", "update_at", "update_by").Find(&objs).Error
	if err == nil {
		for i := 0; i < len(objs); i++ {
			objs[i].TagsLst = strings.Fields(objs[i].Tags)
		}
	}

	return objs, err
}

func DashboardGetsByIds(ctx *ctx.Context, ids []int64) ([]Dashboard, error) {
	if len(ids) == 0 {
		return []Dashboard{}, nil
	}

	var lst []Dashboard
	err := DB(ctx).Where("id in ?", ids).Order("name").Find(&lst).Error
	return lst, err
}

func DashboardGetAll(ctx *ctx.Context) ([]Dashboard, error) {
	var lst []Dashboard
	err := DB(ctx).Find(&lst).Error
	return lst, err
}
