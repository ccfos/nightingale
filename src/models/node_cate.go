package models

import "fmt"

type NodeCate struct {
	Id        int64  `json:"id"`
	Ident     string `json:"ident"`
	Name      string `json:"name"`
	IconColor string `json:"icon_color"`
	Protected int    `json:"protected"`
}

func NodeCateNew(objPtr *NodeCate) error {
	cnt, err := DB["rdb"].Where("ident=?", objPtr.Ident).Count(new(NodeCate))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("%s already exists", objPtr.Ident)
	}

	_, err = DB["rdb"].Insert(objPtr)
	return err
}

func (nc *NodeCate) Update(cols ...string) error {
	_, err := DB["rdb"].Where("id=?", nc.Id).Cols(cols...).Update(nc)
	return err
}

func NodeCateGet(where string, args ...interface{}) (*NodeCate, error) {
	var obj NodeCate
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

// Del 删除NodeCate的时候无需去管node表，某些node的cate已经被删除了，也没什么大不了
func (nc *NodeCate) Del() error {
	if nc.Protected == 1 {
		return fmt.Errorf("cannot delete protected node-category: " + nc.Ident)
	}
	_, err := DB["rdb"].Where("id=?", nc.Id).Delete(new(NodeCate))
	return err
}

// NodeCateGets 条数非常少，全部返回
func NodeCateGets() ([]NodeCate, error) {
	var objs []NodeCate
	err := DB["rdb"].OrderBy("ident").Find(&objs)
	return objs, err
}
