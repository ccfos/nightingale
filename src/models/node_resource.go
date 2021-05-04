package models

import (
	"fmt"

	"github.com/didi/nightingale/v4/src/common/slice"
)

type NodeResource struct {
	NodeId int64
	ResId  int64
}

func NodeResourceUnbind(nid, rid int64) error {
	_, err := DB["rdb"].Where("node_id=? and res_id=?", nid, rid).Delete(new(NodeResource))
	return err
}

func NodeResourceUnbindByRids(rids []int64) error {
	_, err := DB["rdb"].In("res_id", rids).Delete(new(NodeResource))
	return err
}

func NodeResourceBind(nid, rid int64) error {
	// 判断是否已经存在绑定关系
	total, err := DB["rdb"].Where("node_id=? and res_id=?", nid, rid).Count(new(NodeResource))
	if err != nil {
		return err
	}

	if total > 0 {
		return nil
	}

	// 判断node是否真实存在
	n, err := NodeGet("id=?", nid)
	if err != nil {
		return err
	}

	if n == nil {
		return fmt.Errorf("node[id:%d] not found", nid)
	}

	// 判断resource是否真实存在
	res, err := ResourceGet("id=?", rid)
	if err != nil {
		return err
	}

	if res == nil {
		return fmt.Errorf("resource[id:%d] not found", rid)
	}

	// 绑定节点和资源
	_, err = DB["rdb"].Insert(&NodeResource{
		NodeId: nid,
		ResId:  rid,
	})

	return err
}

func NodeIdsGetByResIds(rids []int64) ([]int64, error) {
	if len(rids) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["rdb"].Table(new(NodeResource)).In("res_id", rids).Select("node_id").Find(&ids)
	if err != nil {
		return ids, err
	}

	return slice.Int64Set(ids), err
}

// ResIdsGetByNodeIds 根据叶子节点获取资源ID列表
func ResIdsGetByNodeIds(nids []int64) ([]int64, error) {
	if len(nids) == 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := DB["rdb"].Table(new(NodeResource)).In("node_id", nids).Select("res_id").Find(&ids)
	if err != nil {
		return ids, err
	}

	return slice.Int64Set(ids), err
}

// ResCountGetByNodeIdsAndWhere 根据叶子节点和Where条件获取资源数量表
func ResCountGetByNodeIdsAndCate(nids []int64, cate string) (int, error) {
	if len(nids) == 0 {
		return 0, nil
	}

	var nodeRess []NodeResource
	err := DB["rdb"].Table(new(NodeResource)).In("node_id", nids).Find(&nodeRess)
	if err != nil {
		return 0, err
	}

	cnt := 0
	for _, res := range nodeRess {
		res, err := ResourceGet("id=?", res.ResId)
		if err != nil {
			return 0, err
		}

		if res == nil {
			continue
		}

		if res.Cate == cate {
			cnt++
		}
	}

	return cnt, nil
}
