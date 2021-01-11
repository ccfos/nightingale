package models

import (
	"fmt"
	"time"
)

type Privilege struct {
	Id          int64     `json:"id"`
	Pid         int64     `json:"pid"`
	Typ         string    `json:"typ"`
	Cn          string    `json:"cn"`
	En          string    `json:"en"`
	Weight      int       `json:"weight"`
	Path        string    `json:"path"`
	Leaf        int       `json:"leaf"`
	LastUpdater string    `json:"last_updater"`
	LastUpdated time.Time `json:"last_updated"`
}

func PrivilegeGet(where string, args ...interface{}) (*Privilege, error) {
	var obj Privilege
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func PrivilegeGets(where string, args ...interface{}) ([]Privilege, error) {
	var objs []Privilege
	err := DB["rdb"].Where(where, args...).Find(&objs)
	return objs, err
}

func (p *Privilege) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", p.Id).Cols(cols...).Update(p)
	return err
}

func (p *Privilege) Save() error {
	_, err := DB["rdb"].InsertOne(p)
	return err
}

// 批量添加
func PrivilegeAdds(ps []*Privilege) error {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	for _, p := range ps {
		// path 已存在不允许插入
		pexist, err := PrivilegeGets("path=?", p.Path)
		if err != nil {
			if err != nil {
				session.Rollback()
				return err
			}
		}

		if len(pexist) > 0 {
			session.Rollback()
			return fmt.Errorf("privilege[%s] is exist", p.Path)
		}

		_, err = session.InsertOne(p)
		if err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

// 批量删除
func PrivilegeDels(ids []int64) error {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	for _, id := range ids {
		// 下面有子节点的话不允许删除
		p, err := PrivilegeGet("id=?", id)
		if err != nil {
			session.Rollback()
			return err
		}

		if p == nil {
			session.Rollback()
			return fmt.Errorf("privilege[%d] is nil", id)
		}

		ps, err := PrivilegeGets("pid=?", p.Id)
		if err != nil {
			session.Rollback()
			return err
		}

		if len(ps) > 0 {
			session.Rollback()
			return fmt.Errorf("privilege[%s] has child privilege", p.Path)
		}

		_, err = session.Where("id=?", id).Delete(new(Privilege))
		if err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}
