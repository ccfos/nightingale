package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/models"
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

type hostRegisterForm struct {
	SN      string                 `json:"sn"`
	IP      string                 `json:"ip"`
	Ident   string                 `json:"ident"`
	Name    string                 `json:"name"`
	Cate    string                 `json:"cate"`
	UniqKey string                 `json:"uniqkey"`
	Fields  map[string]interface{} `json:"fields"`
	Digest  string                 `json:"digest"`
}

func (f hostRegisterForm) Validate() {
	if f.IP == "" {
		bomb("ip is blank")
	}

	if f.UniqKey == "" {
		bomb("uniqkey is blank")
	}

	if f.Digest == "" {
		bomb("digest is blank")
	}
}

// mapKeyClear map key clear
func mapKeyClear(src map[string]interface{}, save map[string]struct{}) {
	var dels []string
	for k := range src {
		if _, ok := save[k]; !ok {
			dels = append(dels, k)
		}
	}

	for i := 0; i < len(dels); i++ {
		delete(src, dels[i])
	}
}

// agent主动上报注册信息
func v1HostRegister(c *gin.Context) {
	var f hostRegisterForm
	bind(c, &f)
	f.Validate()

	oldFields := make(map[string]interface{}, len(f.Fields))
	for k, v := range f.Fields {
		oldFields[k] = v
	}

	uniqValue := ""

	if f.UniqKey == "sn" {
		uniqValue = f.SN
	}

	if f.UniqKey == "ip" {
		uniqValue = f.IP
	}

	if f.UniqKey == "ident" {
		uniqValue = f.Ident
	}

	if f.UniqKey == "name" {
		uniqValue = f.Name
	}

	if uniqValue == "" {
		bomb("%s is blank", f.UniqKey)
	}

	cacheKey := "/host/info/" + f.UniqKey + "/" + uniqValue

	var val string
	if err := cache.Get(cacheKey, &val); err == nil {
		if f.Digest == val {
			// 说明客户端采集到的各个字段信息并无变化，无需更新DB
			renderMessage(c, nil)
			return
		}
	} else {
		if err.Error() != cache.ErrCacheMiss.Error() {
			msg := "get cache err"
			logger.Error(err)
			renderMessage(c, msg)
			return
		}
	}

	host, err := models.HostGet(f.UniqKey+" = ?", uniqValue)
	dangerous(err)

	hFixed := map[string]struct{}{
		"cpu":  struct{}{},
		"mem":  struct{}{},
		"disk": struct{}{},
	}

	mapKeyClear(f.Fields, hFixed)

	if host == nil {
		msg := "create host failed"
		host, err = models.HostNew(f.SN, f.IP, f.Ident, f.Name, f.Cate, f.Fields)
		if err != nil {
			logger.Error(err)
			renderMessage(c, msg)
			return
		}

		if host == nil {
			logger.Errorf("%s, report info:%v", msg, f)
			renderMessage(c, msg)
			return
		}
	} else {
		f.Fields["sn"] = f.SN
		f.Fields["ip"] = f.IP
		f.Fields["ident"] = f.Ident
		f.Fields["name"] = f.Name
		f.Fields["cate"] = f.Cate
		f.Fields["clock"] = time.Now().Unix()

		err = host.Update(f.Fields)
		if err != nil {
			logger.Error(err)
			msg := "update host err"
			renderMessage(c, msg)
			return
		}
	}

	if v, ok := oldFields["tenant"]; ok {
		vStr := v.(string)
		if vStr != "" {
			err = models.HostUpdateTenant([]int64{host.Id}, vStr)
			if err != nil {
				logger.Error(err)
				msg := "update host tenant err"
				renderMessage(c, msg)
				return
			}

			err = models.ResourceRegister([]models.Host{*host}, vStr)
			if err != nil {
				logger.Error(err)
				msg := "register resource err"
				renderMessage(c, msg)
				return
			}
		}
	}

	if host.Tenant != "" {
		// 已经分配给某个租户了，那肯定对应某个resource，需要更新resource的信息
		res, err := models.ResourceGet("uuid=?", fmt.Sprintf("host-%d", host.Id))
		dangerous(err)

		if res == nil {
			// 数据不干净，ams里有这个host，而且是已分配状态，但是resource表里没有，重新注册一下
			dangerous(models.ResourceRegister([]models.Host{*host}, host.Tenant))

			// 注册完了，重新查询一下试试
			res, err = models.ResourceGet("uuid=?", fmt.Sprintf("host-%d", host.Id))
			dangerous(err)

			if res == nil {
				bomb("resource register fail, unknown error")
			}
		}

		res.Ident = f.Ident
		res.Name = f.Name
		res.Cate = f.Cate

		mapKeyClear(f.Fields, hFixed)

		js, err := json.Marshal(f.Fields)
		dangerous(err)

		res.Extend = string(js)

		dangerous(res.Update("ident", "name", "cate", "extend"))
	}

	var objs []models.HostFieldValue
	for k, v := range oldFields {
		if k == "tenant" {
			continue
		}

		if _, ok := hFixed[k]; !ok {
			tmp := models.HostFieldValue{HostId: host.Id, FieldIdent: k, FieldValue: v.(string)}
			objs = append(objs, tmp)
		}
	}

	if len(objs) > 0 {
		err = models.HostFieldValuePuts(host.Id, objs)
		dangerous(err)
	}

	err = cache.Set(cacheKey, f.Digest, cache.DEFAULT)
	if err != nil {
		msg := "set cache err"
		logger.Error(err)
		renderMessage(c, msg)
		return
	}

	renderMessage(c, nil)
}
