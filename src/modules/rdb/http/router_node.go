package http

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
)

func nodeGet(c *gin.Context) {
	node := Node(urlParamInt64(c, "id"))
	node.FillAdmins()
	renderData(c, node, nil)
}

//使用场景：节点被删除了，但还是需要查询节点来补全信息
func nodeIncludeTrashGet(c *gin.Context) {
	nid := urlParamInt64(c, "id")
	realNode, err := models.NodeGet("id=?", nid)
	dangerous(err)
	if realNode != nil {
		realNode.FillAdmins()
		renderData(c, realNode, nil)
		return
	}

	var node *models.Node
	nodesInTrash, err := models.NodeTrashGetByIds([]int64{nid})
	dangerous(err)
	if len(nodesInTrash) == 1 {
		nodeInTrash := nodesInTrash[0]
		node = &models.Node{
			Id:          nid,
			Pid:         nodeInTrash.Pid,
			Ident:       nodeInTrash.Ident,
			Name:        nodeInTrash.Name,
			Note:        nodeInTrash.Note,
			Path:        nodeInTrash.Path,
			Leaf:        nodeInTrash.Leaf,
			Cate:        nodeInTrash.Cate,
			IconColor:   nodeInTrash.IconColor,
			IconChar:    nodeInTrash.IconChar,
			Proxy:       nodeInTrash.Proxy,
			Creator:     nodeInTrash.Creator,
			LastUpdated: nodeInTrash.LastUpdated,
		}
	}

	renderData(c, node, nil)
}

func nodeGets(c *gin.Context) {
	cate := queryStr(c, "cate", "")
	withInner := queryInt(c, "inner", 0)
	ids := queryStr(c, "ids", "")

	where := ""
	param := []interface{}{}
	if cate != "" {
		where += "cate = ?"
		param = append(param, cate)
	}

	if withInner == 0 {
		if where != "" {
			where += " and "
		}
		where += "path not like ?"
		param = append(param, "inner")
	}

	if ids != "" {
		if where != "" {
			where += " and "
		}
		where += "id in (" + ids + ")"
	}

	nodes, err := models.NodeGets(where, param...)
	for i := 0; i < len(nodes); i++ {
		nodes[i].FillAdmins()
	}

	renderData(c, nodes, err)
}

type nodeForm struct {
	Pid      int64   `json:"pid"`
	Ident    string  `json:"ident"`
	Name     string  `json:"name"`
	Note     string  `json:"note"`
	Leaf     int     `json:"leaf"`
	Cate     string  `json:"cate"`
	Proxy    int     `json:"proxy"`
	AdminIds []int64 `json:"admin_ids"`
}

func (f nodeForm) Validate() {
	if f.Pid < 0 {
		bomb("arg[pid] invalid")
	}

	if !str.IsMatch(f.Ident, `^[a-z0-9\-_]+$`) {
		bomb("ident legal characters: [a-z0-9_-]")
	}

	if len(f.Ident) >= 32 {
		bomb("ident length should be less than 32")
	}

	if f.Leaf != 0 && f.Leaf != 1 {
		bomb("arg[leaf] invalid")
	}
}

func nodePost(c *gin.Context) {
	var f nodeForm
	bind(c, &f)
	f.Validate()

	me := loginUser(c)

	if f.Pid == 0 {
		// 只有超管才能创建租户
		if !me.IsRooter() {
			bomb("no privilege")
		}

		// 租户节点，租户节点已经设置为protected了，理论上不能被删除
		nc, err := models.NodeCateGet("ident=?", "tenant")
		dangerous(err)

		if nc == nil {
			bomb("node-category[tenant] not found")
		}

		node := &models.Node{
			Pid:       0,
			Ident:     f.Ident,
			Name:      f.Name,
			Path:      f.Ident,
			Leaf:      0,
			Cate:      "tenant",
			IconColor: nc.IconColor,
			IconChar:  "T",
			Proxy:     0,
			Note:      f.Note,
			Creator:   me.Username,
		}

		// 保存node到数据库
		dangerous(models.NodeNew(node, f.AdminIds))

		go models.OperationLogNew(me.Username, "node", node.Id, fmt.Sprintf("NodeCreate path: %s, name: %s", node.Path, node.Name))

		// 把节点详情返回，便于前端易用性处理
		renderData(c, node, nil)
	} else {
		// 非租户节点
		parent, err := models.NodeGet("id=?", f.Pid)
		dangerous(err)

		if parent == nil {
			bomb("arg[pid] invalid, no such parent node")
		}

		me.CheckPermByNode(parent, "rdb_node_create")

		if parent.Proxy > 0 {
			bomb("node is managed by other system")
		}

		child, err := parent.CreateChild(f.Ident, f.Name, f.Note, f.Cate, me.Username, f.Leaf, f.Proxy, f.AdminIds)
		if err == nil {
			go models.OperationLogNew(me.Username, "node", child.Id, fmt.Sprintf("NodeCreate path: %s, name: %s", child.Path, child.Name))
		}
		renderData(c, child, err)
	}
}

func nodePut(c *gin.Context) {
	var f nodeForm
	bind(c, &f)

	id := urlParamInt64(c, "id")
	node := Node(id)

	me := loginUser(c)
	me.CheckPermByNode(node, "rdb_node_modify")

	// 即使是第三方系统创建的节点，也可以修改，只是改个名字、备注、类别、管理员，没啥大不了的
	// 第三方系统主要是管理下面的资源的挂载
	//if node.Proxy > 0 {
	//	bomb("node is managed by other system")
	//}

	if node.Cate == "tenant" && node.Cate != f.Cate {
		bomb("cannot modify tenant's node-category")
	}

	if node.Pid > 0 && f.Cate == "tenant" {
		bomb("cannot modify node-category to tenant")
	}

	err := node.Modify(f.Name, f.Cate, f.Note, f.AdminIds)
	go models.OperationLogNew(me.Username, "node", node.Id, fmt.Sprintf("NodeModify path: %s, name: %s clientIP: %s", node.Path, node.Name, c.ClientIP()))
	renderData(c, node, err)
}

func nodeDel(c *gin.Context) {
	id := urlParamInt64(c, "id")
	node := Node(id)
	me := loginUser(c)
	me.CheckPermByNode(node, "rdb_node_delete")

	dangerous(node.Del())
	go models.OperationLogNew(me.Username, "node", node.Id, fmt.Sprintf("NodeDelete path: %s, name: %s clientIP: %s", node.Path, node.Name, c.ClientIP()))
	renderMessage(c, nil)
}
