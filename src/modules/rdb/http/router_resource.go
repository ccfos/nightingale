package http

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
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

type containerSyncForm struct {
	Name  string                     `json:"name" binding:"required"`
	Type  string                     `json:"type" binding:"required"`
	Items []v1ContainersRegisterItem `json:"items"`
}

func v1ContainerSyncPost(c *gin.Context) {
	var sf containerSyncForm
	bind(c, &sf)

	var (
		uuids []string
	)

	list, err := models.ResourceGets("labels like ?",
		fmt.Sprintf("%%,res_type=%s,res_name=%s%%", sf.Type, sf.Name))
	dangerous(err)

	for _, l := range list {
		uuids = append(uuids, l.UUID)
	}

	dangerous(models.ResourceUnregister(uuids))

	count := len(sf.Items)
	if count == 0 {
		renderMessage(c, "")
		return
	}

	resourceHttpRegister(count, sf.Items)

	renderMessage(c, "")
}

type resourceNotePutForm struct {
	Ids  []int64 `json:"ids" binding:"required"`
	Note string  `json:"note"`
}

type resourceLabelsPutForm struct {
	Ids    []int64 `json:"ids" binding:"required"`
	Labels string  `json:"labels"`
}

func (f resourceNotePutForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

func (f resourceLabelsPutForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

func resourceHttpRegister(count int, items []v1ContainersRegisterItem) {
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
			bomb("resources not found by %s", "uuic")
		}
	} else if f.Field == "ident" {
		ids, err = models.ResourceIdsByIdents(f.Items)
		dangerous(err)
		if len(ids) == 0 {
			bomb("resources not found by %s", "ident")
		}
	} else if f.Field == "id" {
		ids = str.IdsInt64(strings.Join(f.Items, ","))
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

func resourceUnderNodeLabelsPut(c *gin.Context) {
	var f resourceLabelsPutForm
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

		if res.Labels == f.Labels {
			continue
		}

		res.Labels = f.Labels
		dangerous(res.Update("labels"))
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

var needSourceList = []string{"physical", "virtual", "redis", "mongo", "mysql", "container", "sw", "volume"}

func renderNodeResourcesCountByCate(c *gin.Context) {
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

	aggDat := make(map[string]int, len(needSourceList))
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

func renderAllResourcesCountByCate(c *gin.Context) {
	aggDat := make(map[string]int, len(needSourceList))
	ress, err := models.ResourceGets("", nil)
	if err != nil {
		logger.Error(err)
		dangerous(err)
	}

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

// 租户项目粒度资源排行
type resourceRank struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func tenantResourcesCountRank(c *gin.Context) {
	resCate := queryStr(c, "resource_cate", "virtual")
	top := queryInt(c, "top", 0)
	if top < 0 {
		dangerous(fmt.Errorf("param top < 0"))
	}

	tenantNode, err := models.NodeGets("cate=?", "tenant")
	dangerous(err)

	tenantNodeLen := len(tenantNode)
	tenantNodeName := make(map[string]string, tenantNodeLen)
	for _, node := range tenantNode {
		if node.Ident != "" && node.Name != "" {
			tenantNodeName[node.Ident] = node.Name
		}
	}

	ress, err := models.ResourceGets("cate=?", resCate)
	dangerous(err)

	resMap := make(map[string]int, 50)
	for _, res := range ress {
		tenant := res.Tenant
		if tenant != "" {
			if _, ok := resMap[tenant]; !ok {
				resMap[tenant] = 0
			}

			resMap[tenant]++
		}
	}

	var ret []*resourceRank
	for k, v := range resMap {
		tR := new(resourceRank)
		name, ok := tenantNodeName[k]
		if !ok {
			name = k
		}
		tR.Name = name
		tR.Count = v

		ret = append(ret, tR)
	}

	retLen := len(ret)
	if retLen > 0 {
		sort.Slice(ret, func(i, j int) bool { return ret[i].Count > ret[j].Count })
	}

	if top == 0 {
		renderData(c, ret, nil)
		return
	}

	if retLen > top {
		renderData(c, ret[:top], nil)
		return
	}

	renderData(c, ret, nil)
}

func projectResourcesCountRank(c *gin.Context) {
	resCate := queryStr(c, "resource_cate", "virtual")
	top := queryInt(c, "top", 0)
	if top < 0 {
		dangerous(fmt.Errorf("param top < 0"))
	}

	// 获取全部project
	projectNodes, err := models.NodeGets("cate=?", "project")
	dangerous(err)

	projectNodesLen := len(projectNodes)
	workerNum := 50
	if projectNodesLen < workerNum {
		workerNum = projectNodesLen
	}

	worker := make(chan struct{}, workerNum) // 控制 goroutine 并发数
	dataChan := make(chan *resourceRank, projectNodesLen)

	done := make(chan struct{}, 1)
	resp := make([]*resourceRank, 0)
	go func() {
		defer func() { done <- struct{}{} }()
		for d := range dataChan {
			resp = append(resp, d)
		}
	}()

	for _, pN := range projectNodes {
		worker <- struct{}{}
		go singleProjectResCount(pN.Id, resCate, worker,
			dataChan)
	}

	// 等待所有 goroutine 执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}
	close(dataChan)

	// 等待所有 dataChan 被消费完
	<-done

	//整理resp中数据
	respLen := len(resp)
	if respLen > 0 {
		sort.Slice(resp, func(i, j int) bool { return resp[i].Count > resp[j].Count })
	}

	if top == 0 {
		renderData(c, resp, nil)
		return
	}

	if respLen > top {
		renderData(c, resp[:top], nil)
		return
	}

	renderData(c, resp, nil)
}

func singleProjectResCount(id int64, resCate string, worker chan struct{}, dataChan chan *resourceRank) {
	defer func() {
		<-worker
	}()

	node, err := models.NodeGet("id=?", id)
	if err != nil {
		logger.Error(err)
		return
	}

	if node == nil {
		logger.Errorf("node id %d is nil", id)
		return
	}

	leafIds, err := node.LeafIds()
	if err != nil {
		logger.Error(err)
		return
	}

	cnt, err := models.ResCountGetByNodeIdsAndCate(leafIds, resCate)
	if err != nil {
		logger.Error(err)
		return
	}

	data := new(resourceRank)
	nodeName := node.Name
	if nodeName != "" {
		data.Name = nodeName
	} else {
		data.Name = node.Ident
	}
	data.Count = cnt

	dataChan <- data
}
