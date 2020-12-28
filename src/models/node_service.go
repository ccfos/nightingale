package models

import (
	"github.com/toolkits/pkg/slice"
)

func TreeUntilProjectsGetByNid(nid int64) ([]Node, error) {
	nodes, err := NodeByIds([]int64{nid})
	if err != nil {
		return []Node{}, err
	}

	ret, err := PermNodes(nodes)
	if err != nil {
		return ret, err
	}

	cnt := len(ret)
	all := make(map[string]Node, cnt)
	for i := 0; i < cnt; i++ {
		all[ret[i].Path] = ret[i]
	}

	// 只取project（含）以上的部分
	var oks []Node

	set := make(map[string]struct{})
	for i := 0; i < cnt; i++ {
		if ret[i].Cate == "project" {
			paths := Paths(ret[i].Path)
			for _, path := range paths {
				if _, has := set[path]; has {
					continue
				}

				set[path] = struct{}{}
				oks = append(oks, all[path])
			}
		}
	}

	return oks, err
}

func TreeUntilProjectsGetByUser(user *User) ([]Node, error) {
	ret, err := UserPermNodes(user)
	if err != nil {
		return ret, err
	}

	cnt := len(ret)
	all := make(map[string]Node, cnt)
	for i := 0; i < cnt; i++ {
		all[ret[i].Path] = ret[i]
	}

	// 只取project（含）以上的部分
	var oks []Node

	set := make(map[string]struct{})
	for i := 0; i < cnt; i++ {
		if ret[i].Cate == "project" {
			paths := Paths(ret[i].Path)
			for _, path := range paths {
				if _, has := set[path]; has {
					continue
				}

				set[path] = struct{}{}
				oks = append(oks, all[path])
			}
		}
	}

	return oks, err
}

func UserPermNodes(me *User) ([]Node, error) {
	var ret []Node
	var err error

	if me.IsRooter() {
		// 我是超管，自然可以看到整棵树
		ret, err = NodeGets("")
		return ret, err
	}

	// 我可能是某个节点的管理员，或者在某些节点有授权
	nids1, err := NodeIdsIamAdmin(me.Id)
	if err != nil {
		return ret, err
	}

	nids2, err := NodeIdsBindingUsername(me.Username)
	if err != nil {
		return ret, err
	}

	// nodes 是直接与我相关的节点，要返回树的话，下面的子节点、上面的关联父祖节点都要返回
	nodes, err := NodeByIds(slice.MergeInt64(nids1, nids2))
	if err != nil {
		return ret, err
	}

	ret, err = PermNodes(nodes)
	return ret, err
}
