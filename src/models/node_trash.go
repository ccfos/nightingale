package models

import (
	"fmt"
	"time"
)

type NodeTrash struct {
	Id          int64     `json:"id"`
	Pid         int64     `json:"pid"`
	Ident       string    `json:"ident"`
	Name        string    `json:"name"`
	Note        string    `json:"note"`
	Path        string    `json:"path"`
	Leaf        int       `json:"leaf"`
	Cate        string    `json:"cate"`
	IconColor   string    `json:"icon_color"`
	IconChar    string    `json:"icon_char"`
	Proxy       int       `json:"proxy"`
	Creator     string    `json:"creator"`
	LastUpdated time.Time `json:"last_updated" xorm:"<-"`
}

func NodeTrashTotal(query string) (int64, error) {
	if query != "" {
		q := "%" + query + "%"
		return DB["rdb"].Where("path like ? or name like ?", q, q).Count(new(NodeTrash))
	}

	return DB["rdb"].Count(new(NodeTrash))
}

func NodeTrashGets(query string, limit, offset int) ([]NodeTrash, error) {
	session := DB["rdb"].OrderBy("path").Limit(limit, offset)
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("path like ? or name like ?", q, q)
	}

	var objs []NodeTrash
	err := session.Find(&objs)
	return objs, err
}

func NodeTrashGetByIds(ids []int64) ([]NodeTrash, error) {
	if len(ids) == 0 {
		return []NodeTrash{}, nil
	}

	var objs []NodeTrash
	err := DB["rdb"].In("id", ids).Find(&objs)
	return objs, err
}

// 从node_trash表回收部分node，前端一个一个操作，也可以同一层级同时操作
// 之前的父节点可能已经挪动过，所以回收的时候要注意重新更新path信息
func NodeTrashRecycle(ids []int64) error {
	nts, err := NodeTrashGetByIds(ids)
	if err != nil {
		return err
	}

	count := len(nts)
	if count == 0 {
		return fmt.Errorf("nodes not found, refresh and retry")
	}

	nodes := make([]Node, 0, count)

	for i := 0; i < count; i++ {
		node := Node{
			Id:        nts[i].Id,
			Pid:       nts[i].Pid,
			Ident:     nts[i].Ident,
			Name:      nts[i].Name,
			Note:      nts[i].Note,
			Leaf:      nts[i].Leaf,
			Cate:      nts[i].Cate,
			IconColor: nts[i].IconColor,
			IconChar:  nts[i].IconChar,
			Proxy:     nts[i].Proxy,
			Creator:   nts[i].Creator,
		}

		if node.Cate != "tenant" {
			// 判断这个节点的父节点是否存在
			parent, err := NodeGet("id=?", nts[i].Pid)
			if err != nil {
				return err
			}

			if parent == nil {
				return fmt.Errorf("parent node not found")
			}

			node.Path = parent.Path + "." + nts[i].Ident
		} else {
			// 租户节点，自然是没有父节点的
			node.Path = nts[i].Path
		}

		nodes = append(nodes, node)
	}

	session := DB["rdb"].NewSession()
	defer session.Close()

	if err = session.Begin(); err != nil {
		return err
	}

	for i := 0; i < len(nodes); i++ {
		if _, err = session.InsertOne(nodes[i]); err != nil {
			session.Rollback()
			return err
		}
	}

	if _, err = session.In("id", ids).Delete(new(NodeTrash)); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}
