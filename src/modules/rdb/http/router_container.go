package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

type v1ContainersRegisterItem struct {
	UUID   string `json:"uuid"`
	Ident  string `json:"ident"`
	Name   string `json:"name"`
	Labels string `json:"labels"`
	Extend string `json:"extend"`
	Cate   string `json:"cate"`
	NID    int64  `json:"nid"`
}

func (f v1ContainersRegisterItem) Validate() {
	if f.Cate == "" {
		bomb("cate is blank")
	}

	if f.UUID == "" {
		bomb("uuid is blank")
	}

	if f.Ident == "" {
		bomb("ident is blank")
	}
}

func convertItems(items []v1ContainersRegisterItem) (newItems []map[string]interface{}) {
	for _, i := range items {
		newItems = append(newItems, map[string]interface{}{
			"uuid":   i.UUID,
			"ident":  i.Ident,
			"name":   i.Name,
			"labels": i.Labels,
			"extend": i.Extend,
			"cate":   i.Cate,
			"nid":    i.NID,
		})
	}
	return
}

func v1ContainersBindPost(c *gin.Context) {
	var items []v1ContainersRegisterItem
	bind(c, &items)

	count := len(items)
	if count == 0 {
		bomb("items empty")
	}

	for i := 0; i < count; i++ {
		items[i].Validate()

		node := Node(items[i].NID)
		if node.Leaf != 1 {
			bomb("node not leaf")
		}

		res, err := models.ResourceGet("uuid=?", items[i].UUID)
		dangerous(err)

		if res != nil {
			// 这个资源之前就已经存在过了，这次可能是更新了部分字段
			res.Name = items[i].Name
			res.Labels = items[i].Labels
			res.Extend = items[i].Extend
			dangerous(res.Update("name", "labels", "extend"))
		} else {
			// 之前没有过这个资源，在RDB注册这个资源
			res = new(models.Resource)
			res.UUID = items[i].UUID
			res.Ident = items[i].Ident
			res.Name = items[i].Name
			res.Labels = items[i].Labels
			res.Extend = items[i].Extend
			res.Cate = items[i].Cate
			res.Tenant = node.Tenant()
			dangerous(res.Save())
		}

		dangerous(node.Bind([]int64{res.Id}))

		// 第二个挂载位置：inner.${cate}
		innerCatePath := "inner." + node.Ident
		innerCateNode, err := models.NodeGet("path=?", innerCatePath)
		dangerous(err)

		if innerCateNode == nil {
			innerNode, err := models.NodeGet("path=?", "inner")
			dangerous(err)

			if innerNode == nil {
				bomb("inner node not exists")
			}

			innerCateNode, err = innerNode.CreateChild(node.Ident, node.Name, "", node.Cate, "system", 1, 1, []int64{})
			dangerous(err)
		}

		dangerous(innerCateNode.Bind([]int64{res.Id}))
	}

	renderMessage(c, nil)
}
