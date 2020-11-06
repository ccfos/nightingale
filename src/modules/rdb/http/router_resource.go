package http

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
)

func resourceSearchGet(c *gin.Context) {
	batch := queryStr(c, "batch")
	field := queryStr(c, "field")
	list, err := models.ResourceSearch(batch, field)
	renderData(c, list, err)
}

type resourceNotePutForm struct {
	Ids  []int64 `json:"ids" binding:"required"`
	Note string  `json:"note"`
}

func (f resourceNotePutForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

// 游离资源页面修改备注，超级管理员，或者是租户管理员
func resourceNotePut(c *gin.Context) {
	var f resourceNotePutForm
	bind(c, &f)
	f.Validate()

	me := loginUser(c)

	hasPerm := make(map[string]struct{})

	for i := 0; i < len(f.Ids); i++ {
		res, err := models.ResourceGet("id=?", f.Ids[i])
		dangerous(err)

		if res == nil {
			continue
		}

		if res.Note == f.Note {
			continue
		}

		// 我是超级管理员，自然可以修改
		if me.IsRooter() {
			res.Note = f.Note
			dangerous(res.Update("note"))
			continue
		}

		// 如果这个机器属于某个租户，并且我是租户的管理员，那也可以修改
		// 同时修改一批机器可能都属于同一个租户，所以做个内存缓存
		if _, has := hasPerm[res.Tenant]; has {
			res.Note = f.Note
			dangerous(res.Update("note"))
			continue
		}

		tenantNode, err := models.NodeGet("path=?", res.Tenant)
		dangerous(err)

		if tenantNode == nil {
			bomb("no privilege, resource uuid: %s, ident: %s", res.UUID, res.Ident)
		}

		exists, err := models.NodesAdminExists([]int64{tenantNode.Id}, me.Id)
		dangerous(err)

		if exists {
			hasPerm[res.Tenant] = struct{}{}
			res.Note = f.Note
			dangerous(res.Update("note"))
			continue
		} else {
			bomb("no privilege, resource uuid: %s, ident: %s", res.UUID, res.Ident)
		}
	}

	renderMessage(c, nil)
}

// 查看资源的绑定关系，主要用在页面上查看资源挂载的节点
func resourceBindingsGet(c *gin.Context) {
	idsParam := queryStr(c, "ids", "")
	if len(idsParam) > 0 {
		ids := str.IdsInt64(idsParam)
		bindings, err := models.ResourceBindings(ids)
		renderData(c, bindings, err)
		return
	}

	uuidsParam := queryStr(c, "uuids", "")
	if len(uuidsParam) > 0 {
		ids, err := models.ResourceIdsByUUIDs(strings.Split(uuidsParam, ","))
		dangerous(err)

		bindings, err := models.ResourceBindings(ids)
		renderData(c, bindings, err)
		return
	}

	identsParam := queryStr(c, "idents", "")
	if len(identsParam) > 0 {
		ids, err := models.ResourceIdsByIdents(strings.Split(identsParam, ","))
		dangerous(err)

		bindings, err := models.ResourceBindings(ids)
		renderData(c, bindings, err)
		return
	}

	renderMessage(c, "QueryString invalid")
}

// 游离资源，即已经分配给某个租户，但是没有挂载在服务树的资源
func resourceOrphanGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	batch := queryStr(c, "batch", "")
	field := queryStr(c, "field", "ident")
	tenant := queryStr(c, "tenant", "")

	total, err := models.ResourceOrphanTotal(tenant, query, batch, field)
	dangerous(err)

	list, err := models.ResourceOrphanList(tenant, query, batch, field, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":   list,
		"total":  total,
		"tenant": tenant,
	}, nil)
}

func v1ResourcesUnderNodeGet(c *gin.Context) {
	lids, err := Node(urlParamInt64(c, "id")).LeafIds()
	dangerous(err)

	renderResourcesUnderLeafIds(c, lids)
}

func resourceUnderNodeGet(c *gin.Context) {
	user := loginUser(c)
	node := Node(urlParamInt64(c, "id"))

	lids, err := node.LeafIds()
	dangerous(err)

	// 我在这个节点或者上层节点有权限，我就能看到这个节点下的所有叶子节点挂载的资源
	operation := "rdb_resource_view"
	has, err := user.HasPermByNode(node, operation)
	dangerous(err)
	if has {
		renderResourcesUnderLeafIds(c, lids)
		return
	}

	// 虽然，我没有这个节点的权限，但是我可能有某些子节点的权限
	// 那点击这个节点的时候，可以展示有权限的子节点下面的资源
	// 我是某个子节点的管理员，或者我在某些子节点具有rdb_resource_view权限
	nodeIds1, err := models.NodeIdsIamAdmin(user.Id)
	dangerous(err)

	nodeIds2, err := models.NodeIdsBindingUsernameWithOp(user.Username, operation)
	dangerous(err)

	// 这些节点有哪些是当前节点的子节点？
	nodes, err := models.NodeByIds(slice.MergeInt64(nodeIds1, nodeIds2))
	dangerous(err)

	children := node.FilterMyChildren(nodes)

	// 重置一下这个变量，继续用
	lids = []int64{}
	for i := 0; i < len(children); i++ {
		tmp, err := children[i].LeafIds()
		dangerous(err)

		lids = append(lids, tmp...)
	}

	renderResourcesUnderLeafIds(c, lids)
}

func renderResourcesUnderLeafIds(c *gin.Context, lids []int64) {
	if len(lids) == 0 {
		renderZeroPage(c)
		return
	}

	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	batch := queryStr(c, "batch", "")
	field := queryStr(c, "field", "ident")

	total, err := models.ResourceUnderNodeTotal(lids, query, batch, field)
	dangerous(err)

	list, err := models.ResourceUnderNodeGets(lids, query, batch, field, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type resourceBindForm struct {
	Field string   `json:"field"`
	Items []string `json:"items"`
}

func resourceBindNode(c *gin.Context) {
	node := Node(urlParamInt64(c, "id"))

	if node.Proxy > 0 {
		bomb("node is managed by other system")
	}

	var f resourceBindForm
	bind(c, &f)

	var ids []int64
	var err error
	if f.Field == "uuid" {
		ids, err = models.ResourceIdsByUUIDs(f.Items)
		dangerous(err)
		if len(ids) == 0 {
			bomb("resources not found by uuid")
		}
	} else if f.Field == "ident" {
		ids, err = models.ResourceIdsByIdents(f.Items)
		dangerous(err)
		if len(ids) == 0 {
			bomb("resources not found by ident")
		}
	} else {
		bomb("field[%s] not supported", f.Field)
	}

	loginUser(c).CheckPermByNode(node, "rdb_resource_bind")

	renderMessage(c, node.Bind(ids))
}

func resourceUnbindNode(c *gin.Context) {
	node := Node(urlParamInt64(c, "id"))

	if node.Proxy > 0 {
		bomb("node is managed by other system")
	}

	var f idsForm
	bind(c, &f)

	loginUser(c).CheckPermByNode(node, "rdb_resource_unbind")

	renderMessage(c, node.Unbind(f.Ids))
}

// 这个修改备注信息是在节点下挂载的资源页面，非游离资源页面
func resourceUnderNodeNotePut(c *gin.Context) {
	var f resourceNotePutForm
	bind(c, &f)
	f.Validate()

	node := Node(urlParamInt64(c, "id"))
	loginUser(c).CheckPermByNode(node, "rdb_resource_modify")

	for i := 0; i < len(f.Ids); i++ {
		res, err := models.ResourceGet("id=?", f.Ids[i])
		dangerous(err)

		if res == nil {
			continue
		}

		if res.Note == f.Note {
			continue
		}

		res.Note = f.Note
		dangerous(res.Update("note"))
	}

	renderMessage(c, nil)
}

type v1ResourcesRegisterItem struct {
	UUID   string `json:"uuid"`
	Ident  string `json:"ident"`
	Name   string `json:"name"`
	Labels string `json:"labels"`
	Extend string `json:"extend"`
	Cate   string `json:"cate"`
	NID    int64  `json:"nid"`
}

func (f v1ResourcesRegisterItem) Validate() {
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

// 资源挂在两个地方，一个是所在项目节点下的${cate}节点，一个是inner.${cate}.default节点
// 这俩节点如果不存在则自动创建，并且设置proxy=1，不允许普通用户在这样的节点上挂载/解挂资源
// 资源注册后面要用MQ的方式，不能用HTTP接口，RDB可能挂，数据库可能挂，如果RDB或数据库挂了，子系统就会注册资源失败
// MQ的方式就不怕RDB挂掉了，使用MQ的手工ack方式，只有确认资源正常入库了才发送ack给MQ
func v1ResourcesRegisterPost(c *gin.Context) {
	var items []models.ResourceRegisterItem
	bind(c, &items)

	count := len(items)
	if count == 0 {
		bomb("items empty")
	}

	for i := 0; i < count; i++ {
		errCode, err := models.ResourceRegisterFor3rd(items[i])
		if errCode != 0 {
			dangerous(err)
		}
	}

	renderMessage(c, nil)
}

// 资源如果在管控那边销毁了，就需要告诉RDB
func v1ResourcesUnregisterPost(c *gin.Context) {
	var uuids []string
	bind(c, &uuids)

	dangerous(models.ResourceUnregister(uuids))
	renderMessage(c, nil)
}

type nodeResourcesCountResp struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func renderNodeResourcesCountByCate(c *gin.Context) {
	needSourceList := []string{"physical", "virtual", "redis", "mongo", "mysql", "container", "sw"}

	nodeId := urlParamInt64(c, "id")
	node := Node(nodeId)
	leadIds, err := node.LeafIds()
	dangerous(err)

	limit := 10000
	query := ""
	batch := ""
	field := "ident"

	ress, err := models.ResourceUnderNodeGets(leadIds, query, batch, field, limit, 0)
	dangerous(err)

	aggDat := make(map[string]int, len(ress))
	for _, res := range ress {
		cate := res.Cate
		if cate != "" {
			if _, ok := aggDat[cate]; !ok {
				aggDat[cate] = 0
			}

			aggDat[cate]++
		}
	}

	for _, need := range needSourceList {
		if _, ok := aggDat[need]; !ok {
			aggDat[need] = 0
		}
	}

	var list []*nodeResourcesCountResp
	for n, c := range aggDat {
		ns := new(nodeResourcesCountResp)
		ns.Name = n
		ns.Count = c

		list = append(list, ns)
	}

	renderData(c, list, nil)
}
