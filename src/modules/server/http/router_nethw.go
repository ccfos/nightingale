package http

import (
	"fmt"
	"strings"

	goping "github.com/didi/nightingale/v4/src/common/ping"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/gaochao1/sw"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

type networkHardwareForm struct {
	IPs         string `json:"ips"`
	Cate        string `json:"cate"`
	SnmpVersion string `json:"snmp_version"`
	Auth        string `json:"auth"`
	Region      string `json:"region"`
	Note        string `json:"note"`
}

// agent上报的接口
func networkHardwarePost(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var f networkHardwareForm
	bind(c, &f)
	var ipList []string
	ips := strings.Split(f.IPs, "\n")
	for _, ip := range ips {
		list := sw.ParseIp(ip)

		ipList = append(ipList, list...)
	}

	if config.Config.Nems.CheckTarget {
		ipList = goping.FilterIP(ipList)
	}
	for _, ip := range ipList {
		err := models.NetworkHardwareNew(models.MakeNetworkHardware(ip, f.Cate, f.SnmpVersion, f.Auth, f.Region, f.Note))
		if err != nil {
			logger.Warning(err)
		}
	}

	renderMessage(c, nil)
}

type networkHardwarePutForm struct {
	IP          string `json:"ip"`
	Cate        string `json:"cate"`
	SnmpVersion string `json:"snmp_version"`
	Auth        string `json:"auth"`
	Region      string `json:"region"`
	Note        string `json:"note"`
}

func networkHardwarePut(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var f networkHardwarePutForm
	bind(c, &f)

	id := urlParamInt64(c, "id")
	nh, err := models.NetworkHardwareGet("id=?", id)
	dangerous(err)
	nh.IP = f.IP
	nh.Cate = f.Cate
	nh.SnmpVersion = f.SnmpVersion
	nh.Auth = f.Auth
	nh.Region = f.Region
	nh.Note = f.Note
	err = nh.Update()
	renderData(c, nh, err)
}

type IPForm struct {
	IPs []string `json:"ips"`
}

func networkHardwareByIP(c *gin.Context) {
	var f IPForm
	bind(c, &f)

	renderData(c, models.GetHardwareInfoBy(f.IPs), nil)
}

func networkHardwareGetAll(c *gin.Context) {
	list, err := models.NetworkHardwareList("", 10000000, 0)
	renderData(c, list, err)
}

func networkHardwareGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	total, err := models.NetworkHardwareTotal(query)
	dangerous(err)

	list, err := models.NetworkHardwareList(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type mgrHWNoteForm struct {
	Ids  []int64 `json:"ids" binding:"required"`
	Note string  `json:"note" binding:"required"`
}

func (f mgrHWNoteForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

func mgrHWNotePut(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var f mgrHWNoteForm
	bind(c, &f)
	f.Validate()

	for i := 0; i < len(f.Ids); i++ {
		hw, err := models.NetworkHardwareGet("id=?", f.Ids[i])
		dangerous(err)

		if hw == nil {
			continue
		}

		if hw.Note == f.Note {
			continue
		}

		hw.Note = f.Note
		dangerous(hw.Update("note"))
	}

	renderMessage(c, nil)
}

type mgrHWCateForm struct {
	Ids  []int64 `json:"ids" binding:"required"`
	Cate string  `json:"cate" binding:"required"`
}

func (f mgrHWCateForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

func mgrHWCatePut(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var f mgrHWCateForm
	bind(c, &f)
	f.Validate()

	for i := 0; i < len(f.Ids); i++ {
		hw, err := models.NetworkHardwareGet("id=?", f.Ids[i])
		dangerous(err)

		if hw == nil {
			continue
		}

		if hw.Cate == f.Cate {
			continue
		}

		hw.Cate = f.Cate
		dangerous(hw.Update("cate"))
	}

	renderMessage(c, nil)
}

type mgrHWTenantForm struct {
	Ids    []int64 `json:"ids" binding:"required"`
	Tenant string  `json:"tenant" binding:"required"`
}

func (f mgrHWTenantForm) Validate() {
	if len(f.Ids) == 0 {
		bomb("arg[ids] is empty")
	}
}

func mgrHWTenantPut(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var f mgrHWTenantForm
	bind(c, &f)
	f.Validate()

	var hws []*models.NetworkHardware

	for i := 0; i < len(f.Ids); i++ {
		hw, err := models.NetworkHardwareGet("id=?", f.Ids[i])
		dangerous(err)

		if hw == nil {
			continue
		}

		if hw.Tenant == f.Tenant {
			continue
		}

		hw.Tenant = f.Tenant
		dangerous(hw.Update("tenant"))

		hws = append(hws, hw)
	}

	dangerous(models.NetworkHardwareResourceRegister(hws, f.Tenant))

	renderMessage(c, nil)
}

func networkHardwaresPut(c *gin.Context) {
	var hws []*models.NetworkHardware
	bind(c, &hws)

	for i := 0; i < len(hws); i++ {
		hw, err := models.NetworkHardwareGet("id=?", hws[i].Id)
		dangerous(err)

		if hw == nil {
			continue
		}

		hw.Name = hws[i].Name
		hw.SN = hws[i].SN
		hw.Uptime = hws[i].Uptime
		hw.Info = hws[i].Info

		dangerous(hw.Update("name", "sn", "info", "uptime"))
	}

	renderMessage(c, nil)
}

func hwCateGet(c *gin.Context) {
	cates := []string{"sw", "fw"}
	renderData(c, cates, nil)
}

type hwsDelRev struct {
	Ids []int64 `json:"ids"`
}

func networkHardwareDel(c *gin.Context) {
	username := loginUsername(c)
	can, err := models.UsernameCandoGlobalOp(username, "nems_network_ops")
	dangerous(err)
	if !can {
		bomb("no privilege")
	}

	var recv hwsDelRev
	dangerous(c.ShouldBind(&recv))
	for i := 0; i < len(recv.Ids); i++ {
		err = models.NetworkHardwareDel(recv.Ids[i])
		dangerous(err)
	}

	renderMessage(c, err)
}

func snmpRegionGet(c *gin.Context) {
	renderData(c, config.Config.Monapi.Region, nil)
}

// 从某个租户手上回收资源
func nwBackPut(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()

	loginUser(c).CheckPermGlobal("nems_network_ops")

	count := len(f.Ids)
	for i := 0; i < count; i++ {
		nw, err := models.NetworkHardwareGet("id=?", f.Ids[i])
		dangerous(err)

		if nw == nil {
			logger.Warningf("network hardware %d not exist", f.Ids[i])
			continue
		}

		nw.Tenant = ""
		dangerous(nw.Update("tenant"))
		dangerous(models.ResourceUnregister([]string{nw.SN}))
	}

	renderMessage(c, nil)
}

// 普通用户在批量搜索页面搜索网络设备
func nwSearchGets(c *gin.Context) {
	batch := queryStr(c, "batch")
	field := queryStr(c, "field") // ip,sn,name
	list, err := models.NwSearch(batch, field)
	renderData(c, list, err)
}

// 管理员在主机网络设备管理页面查看列表
func nwGets(c *gin.Context) {
	tenant := queryStr(c, "tenant", "")
	page := queryInt(c, "p", 1)
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	batch := queryStr(c, "batch", "")
	field := queryStr(c, "field", "ip")

	if page < 1 || limit < 1 {
		dangerous(fmt.Errorf("param p or limit < 1"))
	}

	total, err := models.NwTotalForAdmin(tenant, query, batch, field)
	dangerous(err)

	start := (page - 1) * limit
	list, err := models.NwGetsForAdmin(tenant, query, batch, field, limit, start)
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func nwDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()

	loginUser(c).CheckPermGlobal("nems_network_ops")

	count := len(f.Ids)
	for i := 0; i < count; i++ {
		id := f.Ids[i]
		nw, err := models.NetworkHardwareGet("id=?", id)
		dangerous(err)

		if nw == nil {
			logger.Warningf("network hardware %d not exist", id)
			continue
		}

		if nw.Tenant != "" {
			bomb("network_hardware[ip:%s, name:%s] belongs to tenant[:%s], cannot delete", nw.IP, nw.Name, nw.Tenant)
		}

		dangerous(models.ResourceUnregister([]string{nw.SN}))
		dangerous(nw.Del())
	}

	renderMessage(c, nil)
}
