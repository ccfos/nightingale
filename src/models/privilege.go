package models

import (
	"fmt"
	"strings"
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

func subMaxWeight(pid int64) (int, error) {
	var obj Privilege
	has, err := DB["rdb"].Where("pid=?", pid).Desc("weight").Limit(1).Get(&obj)
	if err != nil {
		return -1, err
	}

	if has {
		return obj.Weight, nil
	}

	return -1, nil
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
		pexist, err := PrivilegeGets("typ = ? and path=?", p.Typ, p.Path)
		if err != nil {
			if err != nil {
				session.Rollback()
				return err
			}
		}

		if len(pexist) > 0 {
			session.Rollback()
			return fmt.Errorf("privilege[%s][%s] is exist", p.Typ, p.Path)
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
			continue
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

func PrivilegeUpdates(ps []Privilege, oper string) error {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	for _, p := range ps {
		po, err := PrivilegeGet("id=?", p.Id)
		if err != nil {
			session.Rollback()
			return err
		}

		if po == nil {
			session.Rollback()
			return fmt.Errorf("privilege not exist +%v", p)
		}

		po.Pid = p.Pid
		po.Typ = p.Typ
		po.Cn = p.Cn
		po.En = p.En
		po.Weight = p.Weight
		po.Path = p.Path
		po.Leaf = p.Leaf
		po.LastUpdater = oper
		err = po.Update("pid", "typ", "cn", "en", "weight", "path", "leaf", "last_updater")

		if err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}

func PrivilegeImport(ps []Privilege, oper string) error {
	session := DB["rdb"].NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		return err
	}

	weight := 0
	for _, p := range ps {
		po, err := PrivilegeGet("typ=? and path=?", p.Typ, p.Path)
		if err != nil {
			session.Rollback()
			return err
		}

		if po != nil {
			session.Rollback()
			return fmt.Errorf("privilege[%s][%s] exist", p.Typ, p.Path)
		}

		if strings.Contains(p.Path, ".") {
			pathSlice := strings.Split(p.Path, ".")
			parentSlice := pathSlice[0 : len(pathSlice)-1]
			parentPath := strings.Join(parentSlice, ".")
			parent, err := PrivilegeGet("typ=? and path=?", p.Typ, parentPath)
			if err != nil {
				session.Rollback()
				return err
			}

			if parent == nil {
				session.Rollback()
				return fmt.Errorf("privilege[%s] parent[%s] not exist", p.Path, parentPath)
			}

			// 权重处理
			weight, err = subMaxWeight(parent.Id)
			if err != nil {
				session.Rollback()
				return err
			}

			p.Pid = parent.Id
		} else {
			weight, err = subMaxWeight(0)
			if err != nil {
				session.Rollback()
				return err
			}
		}

		p.Weight = weight + 1
		p.LastUpdater = oper
		err = p.Save()
		if err != nil {
			session.Rollback()
			return err
		}
	}

	return session.Commit()
}
