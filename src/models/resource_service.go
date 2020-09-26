package models

import (
	"fmt"
)

type ResourceRegisterItem struct {
	UUID   string `json:"uuid"`
	Ident  string `json:"ident"`
	Name   string `json:"name"`
	Labels string `json:"labels"`
	Extend string `json:"extend"`
	Cate   string `json:"cate"`
	NID    int64  `json:"nid"`
}

func (i ResourceRegisterItem) Validate() error {
	if i.Cate == "" {
		return fmt.Errorf("cate is blank")
	}

	if i.UUID == "" {
		return fmt.Errorf("uuid is blank")
	}

	if i.Ident == "" {
		return fmt.Errorf("ident is blank")
	}

	return nil
}

// ResourceRegisterFor3rd 用于第三方资源注册 errCode=400: 表示传入的参数有问题 errCode=500: 表示DB出了问题
// 之所以要通过errCode对错误做区分，是因为这个方法同时被同步和异步两种方式调用，上层需要依托这个信息做判断
func ResourceRegisterFor3rd(item ResourceRegisterItem) (errCode int, err error) {
	err = item.Validate()
	if err != nil {
		return 400, err
	}

	node, err := NodeGetById(item.NID)
	if err != nil {
		return 500, err
	}

	if node == nil {
		return 400, fmt.Errorf("node not found")
	}

	if node.Cate != "project" {
		return 400, fmt.Errorf("node not project")
	}

	res, err := ResourceGet("uuid=?", item.UUID)
	if err != nil {
		return 500, err
	}

	if res != nil {
		// 这个资源之前就已经存在过了，这次可能是更新了部分字段
		res.Name = item.Name
		res.Labels = item.Labels
		res.Extend = item.Extend
		err = res.Update("name", "labels", "extend")
		if err != nil {
			return 500, err
		}
	} else {
		// 之前没有过这个资源，在RDB注册这个资源
		res = new(Resource)
		res.UUID = item.UUID
		res.Ident = item.Ident
		res.Name = item.Name
		res.Labels = item.Labels
		res.Extend = item.Extend
		res.Cate = item.Cate
		res.Tenant = node.Tenant()
		err = res.Save()
		if err != nil {
			return 500, err
		}
	}

	// 检查这个资源是否有挂载过，没有的话就补齐挂载关系，这个动作是幂等的
	leafPath := node.Path + "." + item.Cate
	leafNode, err := NodeGet("path=?", leafPath)
	if err != nil {
		return 500, err
	}

	// 第一个挂载位置：项目下面的${cate}节点
	if leafNode == nil {
		leafNode, err = node.CreateChild(item.Cate, item.Cate, "", "resource", "system", 1, 1, []int64{})
		if err != nil {
			return 500, err
		}
	}

	err = leafNode.Bind([]int64{res.Id})
	if err != nil {
		return 500, err
	}

	// 第二个挂载位置：inner.${cate}
	innerCatePath := "inner." + item.Cate
	innerCateNode, err := NodeGet("path=?", innerCatePath)
	if err != nil {
		return 500, err
	}

	if innerCateNode == nil {
		innerNode, err := NodeGet("path=?", "inner")
		if err != nil {
			return 500, err
		}

		if innerNode == nil {
			return 500, fmt.Errorf("inner node not exists, maybe forget init system")
		}

		innerCateNode, err = innerNode.CreateChild(item.Cate, item.Cate, "", "resource", "system", 1, 1, []int64{})
		if err != nil {
			return 500, err
		}
	}

	err = innerCateNode.Bind([]int64{res.Id})
	if err != nil {
		return 500, err
	}

	return 0, nil
}
