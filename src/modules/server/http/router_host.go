package http

import (
	"fmt"
	"strings"

	"github.com/didi/nightingale/v4/src/models"

	"github.com/gin-gonic/gin"
)

// 管理员在主机设备管理页面查看列表
func hostGets(c *gin.Context) {
	tenant := queryStr(c, "tenant", "")
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	batch := queryStr(c, "batch", "")
	field := queryStr(c, "field", "ip")

	total, err := models.HostTotalForAdmin(tenant, query, batch, field)
	dangerous(err)

	list, err := models.HostGetsForAdmin(tenant, query, batch, field, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func hostGet(c *gin.Context) {
	host, err := models.HostGet("id=?", urlParamInt64(c, "id"))
	renderData(c, host, err)
}

// ${ip}::${ident}::${name} 一行一个
func hostPost(c *gin.Context) {
	var arr []string
	bind(c, &arr)

	count := len(arr)
	for i := 0; i < count; i++ {
		fields := strings.Split(arr[i], "::")
		ip := strings.TrimSpace(fields[0])
		if ip == "" {
			bomb("input invalid")
		}
		host := new(models.Host)
		host.IP = ip

		if len(fields) > 1 {
			host.Ident = strings.TrimSpace(fields[1])
		}

		if len(fields) > 2 {
			host.Name = strings.TrimSpace(fields[2])
		}

		dangerous(host.Save())
	}

	renderMessage(c, nil)
}

type idsOrIpsForm struct {
	Ids []int64  `json:"ids"`
	Ips []string `json:"ips"`
}

func (f *idsOrIpsForm) Validate() {
	if len(f.Ids) == 0 {
		if len(f.Ips) == 0 {
			bomb("args invalid")
		}
		ids, err := models.HostIdsByIps(f.Ips)
		dangerous(err)

		f.Ids = ids
	}
}

// 从某个租户手上回收资源
func hostBackPut(c *gin.Context) {
	var f idsOrIpsForm
	bind(c, &f)
	f.Validate()

	loginUser(c).CheckPermGlobal("ams_host_modify")

	count := len(f.Ids)
	for i := 0; i < count; i++ {
		host, err := models.HostGet("id=?", f.Ids[i])
		dangerous(err)

		if host == nil {
			continue
		}

		dangerous(host.Update(map[string]interface{}{"tenant": ""}))
		dangerous(models.ResourceUnregister([]string{fmt.Sprintf("host-%d", f.Ids[i])}))
	}

	renderMessage(c, nil)
}

type hostTenantForm struct {
	Ids    []int64 `json:"ids"`
	Tenant string  `json:"tenant"`
}

func (f *hostTenantForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("ids is empty")
	}

	if f.Tenant == "" {
		bomb("tenant is blank")
	}
}

// 管理员修改主机设备的租户，相当于分配设备
func hostTenantPut(c *gin.Context) {
	var f hostTenantForm
	bind(c, &f)
	f.Validate()

	hosts, err := models.HostByIds(f.Ids)
	dangerous(err)

	if len(hosts) == 0 {
		bomb("hosts is empty")
	}

	loginUser(c).CheckPermGlobal("ams_host_modify")

	err = models.HostUpdateTenant(f.Ids, f.Tenant)
	if err == nil {
		dangerous(models.ResourceRegister(hosts, f.Tenant))
	}

	renderMessage(c, err)
}

type hostNodeForm struct {
	Ids    []int64 `json:"ids"`
	NodeId int64   `json:"nodeid"`
}

func (f *hostNodeForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("ids is empty")
	}

	if f.NodeId == 0 {
		bomb("nodeid is blank")
	}

	if f.NodeId < 0 {
		bomb("nodeid is illegal")
	}
}

// 管理员修改主机设备的节点，相当于挂载设备到节点
func hostNodePut(c *gin.Context) {
	var f hostNodeForm
	bind(c, &f)
	f.Validate()

	loginUser(c).CheckPermGlobal("ams_host_modify")
	node, err := models.NodeGet("id=?", f.NodeId)
	dangerous(err)
	if node == nil {
		bomb("node is nil")
	}

	if node.Leaf != 1 {
		bomb("node is not leaf")
	}

	hosts, err := models.HostByIds(f.Ids)
	dangerous(err)
	if len(hosts) == 0 {
		bomb("hosts is empty")
	}

	for _, h := range hosts {
		if h.Tenant != "" {
			bomb("%s already belongs to %s", h.Name, h.Tenant)
		}
	}

	// 绑定租户
	tenant := node.Tenant()
	err = models.HostUpdateTenant(f.Ids, tenant)
	dangerous(err)
	dangerous(models.ResourceRegister(hosts, tenant))

	// 绑定到节点
	var resUuids []string
	for _, id := range f.Ids {
		idStr := fmt.Sprintf("host-%d", id)
		resUuids = append(resUuids, idStr)
	}
	if len(resUuids) == 0 {
		bomb("res is empty")
	}
	resIds, err := models.ResourceIdsByUUIDs(resUuids)
	dangerous(err)
	if len(resIds) == 0 {
		bomb("res ids is empty")
	}

	renderMessage(c, node.Bind(resIds))
}

type hostNoteForm struct {
	Ids  []int64 `json:"ids"`
	Note string  `json:"note"`
}

// 管理员修改主机设备的备注
func hostNotePut(c *gin.Context) {
	var f hostNoteForm
	bind(c, &f)

	if len(f.Ids) == 0 {
		bomb("ids is empty")
	}

	loginUser(c).CheckPermGlobal("ams_host_modify")

	renderMessage(c, models.HostUpdateNote(f.Ids, f.Note))
}

type hostCateForm struct {
	Ids  []int64 `json:"ids"`
	Cate string  `json:"cate"`
}

// 管理员修改主机设备的类别
func hostCatePut(c *gin.Context) {
	var f hostCateForm
	bind(c, &f)

	if len(f.Ids) == 0 {
		bomb("ids is empty")
	}

	loginUser(c).CheckPermGlobal("ams_host_modify")

	renderMessage(c, models.HostUpdateCate(f.Ids, f.Cate))
}

// 删除某个机器，比如机器过保了，删除机器这个动作很大，需要慎重
// 先检查tenant字段是否为空，如果不为空，说明机器仍然在业务线使用，拒绝删除
// 管理员可以先点【回收】从业务线回收机器，unregister之后tenant字段为空即可删除
func hostDel(c *gin.Context) {
	var f idsOrIpsForm
	bind(c, &f)
	f.Validate()

	loginUser(c).CheckPermGlobal("ams_host_delete")

	count := len(f.Ids)
	for i := 0; i < count; i++ {
		id := f.Ids[i]

		host, err := models.HostGet("id=?", id)
		dangerous(err)

		if host == nil {
			continue
		}

		if host.Tenant != "" {
			bomb("host[ip:%s, name:%s] belongs to tenant[:%s], cannot delete", host.IP, host.Name, host.Tenant)
		}

		dangerous(models.ResourceUnregister([]string{fmt.Sprintf("host-%d", host.Id)}))
		dangerous(host.Del())
	}

	renderMessage(c, nil)
}

// 普通用户在批量搜索页面搜索设备
func hostSearchGets(c *gin.Context) {
	batch := queryStr(c, "batch")
	field := queryStr(c, "field") // ip,sn,name
	list, err := models.HostSearch(batch, field)
	renderData(c, list, err)
}

// agent主动上报注册信息
func v1HostRegister(c *gin.Context) {
	var f models.HostRegisterForm
	bind(c, &f)
	f.Validate()

	err := models.HostRegister(f)
	renderMessage(c, err)
}
