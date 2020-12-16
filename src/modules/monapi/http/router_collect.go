package http

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/scache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type CollectRecv struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

//此处实现需要重构
func collectPost(c *gin.Context) {
	creator := loginUsername(c)
	var recv []CollectRecv
	errors.Dangerous(c.ShouldBind(&recv))

	for _, obj := range recv {
		switch obj.Type {
		case "port":
			collect := new(models.PortCollect)

			b, err := json.Marshal(obj.Data)
			if err != nil {
				bomb("marshal body %s err:%v", obj, err)
			}

			err = json.Unmarshal(b, collect)
			if err != nil {
				bomb("unmarshal body %s err:%v", string(b), err)
			}

			can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_create", collect.Nid)
			errors.Dangerous(err)
			if !can {
				bomb("permission deny")
			}

			collect.Creator = creator
			collect.LastUpdator = creator

			nid := collect.Nid
			name := collect.Name

			old, err := models.GetCollectByNameAndNid(obj.Type, name, nid)
			errors.Dangerous(err)
			if old != nil {
				bomb("same stra name %s in node", name)
			}
			errors.Dangerous(models.CreateCollect(obj.Type, creator, collect))

		case "proc":
			collect := new(models.ProcCollect)

			b, err := json.Marshal(obj.Data)
			if err != nil {
				bomb("marshal body %s err:%v", obj, err)
			}

			err = json.Unmarshal(b, collect)
			if err != nil {
				bomb("unmarshal body %s err:%v", string(b), err)
			}

			can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_create", collect.Nid)
			errors.Dangerous(err)
			if !can {
				bomb("permission deny")
			}

			collect.Creator = creator
			collect.LastUpdator = creator

			nid := collect.Nid
			name := collect.Name

			old, err := models.GetCollectByNameAndNid(obj.Type, name, nid)
			errors.Dangerous(err)
			if old != nil {
				bomb("same stra name %s in node", name)
			}
			errors.Dangerous(models.CreateCollect(obj.Type, creator, collect))
		case "log":
			collect := new(models.LogCollect)

			b, err := json.Marshal(obj.Data)
			if err != nil {
				bomb("marshal body %s err:%v", obj, err)
			}

			err = json.Unmarshal(b, collect)
			if err != nil {
				bomb("unmarshal body %s err:%v", string(b), err)
			}

			can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_create", collect.Nid)
			errors.Dangerous(err)
			if !can {
				bomb("permission deny")
			}

			collect.Encode()
			collect.Creator = creator
			collect.LastUpdator = creator

			nid := collect.Nid
			name := collect.Name

			old, err := models.GetCollectByNameAndNid(obj.Type, name, nid)
			errors.Dangerous(err)
			if old != nil {
				bomb("same stra name %s in node", name)
			}
			errors.Dangerous(models.CreateCollect(obj.Type, creator, collect))
		case "plugin":
			collect := new(models.PluginCollect)

			b, err := json.Marshal(obj.Data)
			if err != nil {
				bomb("marshal body %s err:%v", obj, err)
			}

			err = json.Unmarshal(b, collect)
			if err != nil {
				bomb("unmarshal body %s err:%v", string(b), err)
			}

			can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_create", collect.Nid)
			errors.Dangerous(err)
			if !can {
				bomb("permission deny")
			}

			collect.Creator = creator
			collect.LastUpdator = creator

			nid := collect.Nid
			name := collect.Name

			old, err := models.GetCollectByNameAndNid(obj.Type, name, nid)
			errors.Dangerous(err)
			if old != nil {
				bomb("same stra name %s in node", name)
			}
			errors.Dangerous(models.CreateCollect(obj.Type, creator, collect))

		case "api":
			collect := new(models.ApiCollect)

			b, err := json.Marshal(obj.Data)
			if err != nil {
				bomb("marshal body %s err:%v", obj, err)
			}

			err = json.Unmarshal(b, collect)
			if err != nil {
				bomb("unmarshal body %s err:%v", string(b), err)
			}

			can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_create", collect.Nid)
			errors.Dangerous(err)
			if !can {
				bomb("permission deny")
			}

			collect.Encode()
			collect.Creator = creator
			collect.LastUpdator = creator

			nid := collect.Nid
			name := collect.Name

			old, err := models.GetCollectByNameAndNid(obj.Type, name, nid)
			errors.Dangerous(err)
			if old != nil {
				bomb("same stra name %s in node", name)
			}
			errors.Dangerous(models.CreateCollect(obj.Type, creator, collect))

		default:
			bomb("collect type not support")
		}
	}

	renderData(c, "ok", nil)
}

func collectGetByEndpoint(c *gin.Context) {
	collect := scache.CollectCache.GetBy(urlParamStr(c, "endpoint"))
	renderData(c, collect, nil)
}

func collectGet(c *gin.Context) {
	t := mustQueryStr(c, "type")
	nid := mustQueryInt64(c, "id")
	collect, err := models.GetCollectById(t, nid)
	errors.Dangerous(err)

	renderData(c, collect, nil)
}

func collectsGet(c *gin.Context) {
	nid := queryInt64(c, "nid", -1)
	tp := queryStr(c, "type", "")
	var resp []interface{}
	var types []string

	if tp == "" {
		types = []string{"port", "proc", "log", "plugin"}
	} else {
		types = []string{tp}
	}

	if nid == -1 && tp != "" { //没有nid参数，tp不为空
		if tp == "api" {
			collects, err := models.GetApiCollects()
			errors.Dangerous(err)
			for _, c := range collects {
				c.Decode()
				resp = append(resp, c)
			}
		}
	} else {
		nids := []int64{nid}
		for _, t := range types {
			collects, err := models.GetCollectByNid(t, nids)
			if err != nil {
				logger.Warning(t, err)
				continue
			}
			resp = append(resp, collects...)
		}
	}

	renderData(c, resp, nil)
}

func collectPut(c *gin.Context) {
	creator := loginUsername(c)

	var recv CollectRecv
	errors.Dangerous(c.ShouldBind(&recv))

	switch recv.Type {
	case "port":
		collect := new(models.PortCollect)

		b, err := json.Marshal(recv.Data)
		if err != nil {
			bomb("marshal body %s err:%v", recv, err)
		}

		err = json.Unmarshal(b, collect)
		if err != nil {
			bomb("unmarshal body %s err:%v", string(b), err)
		}

		can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_modify", collect.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}

		nid := collect.Nid
		name := collect.Name

		//校验采集是否存在
		obj, err := models.GetCollectById(recv.Type, collect.Id) //id找不到的情况
		if err != nil {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		tmpId := obj.(*models.PortCollect).Id
		if tmpId == 0 {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		collect.Creator = creator
		collect.LastUpdator = creator

		old, err := models.GetCollectByNameAndNid(recv.Type, name, nid)
		errors.Dangerous(err)
		if old != nil && tmpId != old.(*models.PortCollect).Id {
			bomb("same stra name %s in node", name)
		}

		errors.Dangerous(collect.Update())
		renderData(c, "ok", nil)
		return
	case "proc":
		collect := new(models.ProcCollect)

		b, err := json.Marshal(recv.Data)
		if err != nil {
			bomb("marshal body %s err:%v", recv, err)
		}

		err = json.Unmarshal(b, collect)
		if err != nil {
			bomb("unmarshal body %s err:%v", string(b), err)
		}

		can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_modify", collect.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}

		nid := collect.Nid
		name := collect.Name

		//校验采集是否存在
		obj, err := models.GetCollectById(recv.Type, collect.Id) //id找不到的情况
		if err != nil {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		tmpId := obj.(*models.ProcCollect).Id
		if tmpId == 0 {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		can, err = models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_modify", collect.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}

		collect.Creator = creator
		collect.LastUpdator = creator

		old, err := models.GetCollectByNameAndNid(recv.Type, name, nid)
		errors.Dangerous(err)
		if old != nil && tmpId != old.(*models.ProcCollect).Id {
			bomb("same stra name %s in node", name)
		}

		errors.Dangerous(collect.Update())
		renderData(c, "ok", nil)
		return
	case "log":
		collect := new(models.LogCollect)

		b, err := json.Marshal(recv.Data)
		if err != nil {
			bomb("marshal body %s err:%v", recv, err)
		}

		err = json.Unmarshal(b, collect)
		if err != nil {
			bomb("unmarshal body %s err:%v", string(b), err)
		}

		can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_modify", collect.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}

		collect.Encode()
		nid := collect.Nid
		name := collect.Name

		//校验采集是否存在
		obj, err := models.GetCollectById(recv.Type, collect.Id) //id找不到的情况
		if err != nil {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		tmpId := obj.(*models.LogCollect).Id
		if tmpId == 0 {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		collect.Creator = creator
		collect.LastUpdator = creator

		old, err := models.GetCollectByNameAndNid(recv.Type, name, nid)
		errors.Dangerous(err)
		if old != nil && tmpId != old.(*models.LogCollect).Id {
			bomb("same stra name %s in node", name)
		}

		errors.Dangerous(collect.Update())
		renderData(c, "ok", nil)
		return
	case "plugin":
		collect := new(models.PluginCollect)

		b, err := json.Marshal(recv.Data)
		if err != nil {
			bomb("marshal body %s err:%v", recv, err)
		}

		err = json.Unmarshal(b, collect)
		if err != nil {
			bomb("unmarshal body %s err:%v", string(b), err)
		}

		can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_modify", collect.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}

		nid := collect.Nid
		name := collect.Name

		//校验采集是否存在
		obj, err := models.GetCollectById(recv.Type, collect.Id) //id找不到的情况
		if err != nil {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		tmpId := obj.(*models.PluginCollect).Id
		if tmpId == 0 {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		collect.Creator = creator
		collect.LastUpdator = creator

		old, err := models.GetCollectByNameAndNid(recv.Type, name, nid)
		errors.Dangerous(err)
		if old != nil && tmpId != old.(*models.PluginCollect).Id {
			bomb("same stra name %s in node", name)
		}

		errors.Dangerous(collect.Update())
		renderData(c, "ok", nil)
		return
	case "api":
		collect := new(models.ApiCollect)

		b, err := json.Marshal(recv.Data)
		if err != nil {
			bomb("marshal body %s err:%v", recv, err)
		}

		err = json.Unmarshal(b, collect)
		if err != nil {
			bomb("unmarshal body %s err:%v", string(b), err)
		}

		can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_modify", collect.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}

		collect.Encode()
		nid := collect.Nid
		name := collect.Name

		//校验采集是否存在
		obj, err := models.GetCollectById(recv.Type, collect.Id) //id找不到的情况
		if err != nil {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		tmpId := obj.(*models.ApiCollect).Id
		if tmpId == 0 {
			bomb("采集不存在 type:%s id:%d", recv.Type, collect.Id)
		}

		collect.Creator = creator
		collect.LastUpdator = creator

		old, err := models.GetCollectByNameAndNid(recv.Type, name, nid)
		errors.Dangerous(err)
		if old != nil && tmpId != old.(*models.ApiCollect).Id {
			bomb("same stra name %s in node", name)
		}

		errors.Dangerous(collect.Update())
		renderData(c, "ok", nil)
		return

	default:
		bomb("采集类型不合法")
	}

	renderData(c, "ok", nil)
}

type CollectsDelRev struct {
	Type string  `json:"type"`
	Ids  []int64 `json:"ids"`
}

func collectsDel(c *gin.Context) {
	username := loginUsername(c)
	var recv []CollectsDelRev
	errors.Dangerous(c.ShouldBind(&recv))
	for _, obj := range recv {

		for i := 0; i < len(obj.Ids); i++ {
			tmp, err := models.GetCollectById(obj.Type, obj.Ids[i]) //id找不到的情况
			if err != nil {
				bomb("采集不存在 type:%s id:%d", obj.Type, obj.Ids[i])
			}

			if tmp == nil {
				bomb("采集不存在 type:%s id:%d", obj.Type, obj.Ids[i])
			}

			var nid int64
			switch obj.Type {
			case "log":
				nid = tmp.(*models.LogCollect).Nid
			case "proc":
				nid = tmp.(*models.ProcCollect).Nid
			case "port":
				nid = tmp.(*models.PortCollect).Nid
			case "plugin":
				nid = tmp.(*models.PluginCollect).Nid
			}

			can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_collect_delete", int64(nid))
			errors.Dangerous(err)
			if !can {
				bomb("permission deny")
			}
		}

		for i := 0; i < len(obj.Ids); i++ {
			switch obj.Type {
			case "api":
				err := models.DeleteApiCollect(obj.Ids[i])
				errors.Dangerous(err)
			default:
				err := models.DeleteCollectById(obj.Type, username, obj.Ids[i])
				errors.Dangerous(err)
			}

		}
	}

	renderData(c, "ok", nil)
}

type ApiStraRev struct {
	Sid int64 `json:"sid"`
	Cid int64 `json:"cid"`
}

type ApiStraRes struct {
	Has bool  `json:"has"`
	Sid int64 `json:"sid"`
}

func ApiStraGet(c *gin.Context) {
	cid := mustQueryInt64(c, "cid")
	sid, err := models.GetSidByCid(cid)
	var res ApiStraRes
	errors.Dangerous(err)
	if sid != 0 {
		res.Has = true
		res.Sid = sid
	}
	renderData(c, res, nil)
}

func ApiStraPost(c *gin.Context) {
	recv := new(models.ApiCollectSid)
	errors.Dangerous(c.ShouldBind(&recv))
	errors.Dangerous(recv.Add())
	renderData(c, "ok", nil)
	return
}

func ApiRegionGet(c *gin.Context) {
	renderData(c, config.Get().Region, nil)
}

type RegExpCheckDto struct {
	Success bool                `json:"success"`
	Data    []map[string]string `json:"tags"`
}

var RegExpExcludePatition string = "```EXCLUDE```"

func regExpCheck(c *gin.Context) {
	param := make(map[string]string, 0)
	errors.Dangerous(c.ShouldBind(&param))

	ret := &RegExpCheckDto{
		Success: true,
		Data:    make([]map[string]string, 0),
	}

	// 处理时间格式
	if t, ok := param["time"]; !ok || t == "" {
		tmp := map[string]string{"time": "time参数不存在或为空"}
		ret.Data = append(ret.Data, tmp)
	} else {
		timePat, _ := GetPatAndTimeFormat(param["time"])
		if timePat == "" {
			tmp := map[string]string{"time": genErrMsg("时间格式")}
			ret.Data = append(ret.Data, tmp)
		} else {
			suc, tRes, _ := checkRegPat(timePat, param["log"], true)
			if !suc {
				ret.Success = false
				tRes = genErrMsg("时间格式")
			}
			tmp := map[string]string{"time": tRes}
			ret.Data = append(ret.Data, tmp)
		}
	}

	// 计算方式
	calc_method, _ := param["calc_method"]

	// 处理主正则(with exclude)
	if re, ok := param["re"]; !ok || re == "" {
		tmp := map[string]string{"re": "re参数不存在或为空"}
		ret.Data = append(ret.Data, tmp)
	} else {
		// 处理exclude的情况
		exclude := ""
		if strings.Contains(re, RegExpExcludePatition) {
			l := strings.Split(re, RegExpExcludePatition)
			if len(l) >= 2 {
				param["re"] = l[0]
				exclude = l[1]
			}
		}

		// 匹配主正则
		suc, reRes, isSub := checkRegPat(param["re"], param["log"], false)
		if !suc {
			ret.Success = false
			reRes = genErrMsg("主正则")
		}
		if calc_method != "" && calc_method != "cnt" && !isSub {
			ret.Success = false
			reRes = genSubErrMsg("主正则")
		}
		tmp := map[string]string{"主正则": reRes}
		ret.Data = append(ret.Data, tmp)

		// 匹配exclude, 这个不影响失败
		if exclude != "" {
			suc, exRes, _ := checkRegPat(exclude, param["log"], false)
			if !suc {
				//ret.Success = false
				exRes = "未匹配到排除串,请检查是否符合预期"
			}
			tmp := map[string]string{"排除串": exRes}
			ret.Data = append(ret.Data, tmp)
		}
	}

	// 处理tags
	var nonTagKey = map[string]bool{
		"re":          true,
		"log":         true,
		"time":        true,
		"calc_method": true,
	}

	for tagk, pat := range param {
		// 如果不是tag，就继续循环
		if _, ok := nonTagKey[tagk]; ok {
			continue
		}
		suc, tagRes, isSub := checkRegPat(pat, param["log"], false)
		if !suc {
			// 正则错误
			ret.Success = false
			tagRes = genErrMsg(tagk)
		} else if !isSub {
			// 未匹配出子串
			ret.Success = false
			tagRes = genSubErrMsg(tagk)
		} else if includeIllegalChar(tagRes) || includeIllegalChar(tagk) {
			// 保留字报错
			ret.Success = false
			tagRes = genIllegalCharErrMsg()
		}

		tmp := map[string]string{tagk: tagRes}
		ret.Data = append(ret.Data, tmp)
	}

	renderData(c, ret, nil)
}

//根据配置的时间格式，获取对应的正则匹配pattern和time包用的时间格式
func GetPatAndTimeFormat(tf string) (string, string) {
	var pat, timeFormat string
	switch tf {
	case "dd/mmm/yyyy:HH:MM:SS":
		pat = `([012][0-9]|3[01])/[JFMASOND][a-z]{2}/(2[0-9]{3}):([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02/Jan/2006:15:04:05"
	case "dd/mmm/yyyy HH:MM:SS":
		pat = `([012][0-9]|3[01])/[JFMASOND][a-z]{2}/(2[0-9]{3})\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02/Jan/2006 15:04:05"
	case "yyyy-mm-ddTHH:MM:SS":
		pat = `(2[0-9]{3})-(0[1-9]|1[012])-([012][0-9]|3[01])T([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "2006-01-02T15:04:05"
	case "dd-mmm-yyyy HH:MM:SS":
		pat = `([012][0-9]|3[01])-[JFMASOND][a-z]{2}-(2[0-9]{3})\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02-Jan-2006 15:04:05"
	case "yyyy-mm-dd HH:MM:SS":
		pat = `(2[0-9]{3})-(0[1-9]|1[012])-([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "2006-01-02 15:04:05"
	case "yyyy/mm/dd HH:MM:SS":
		pat = `(2[0-9]{3})/(0[1-9]|1[012])/([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "2006/01/02 15:04:05"
	case "yyyymmdd HH:MM:SS":
		pat = `(2[0-9]{3})(0[1-9]|1[012])([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "20060102 15:04:05"
	case "mmm dd HH:MM:SS":
		pat = `[JFMASOND][a-z]{2}\s+([1-9]|[1-2][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "Jan 2 15:04:05"
	case "mmdd HH:MM:SS":
		pat = `(0[1-9]|1[012])([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "0102 15:04:05"
	default:
		logger.Errorf("match time pac failed : [timeFormat:%s]", tf)
		return "", ""
	}
	return pat, timeFormat
}

// 出错信息直接放在body里
func checkRegPat(pat string, log string, origin bool) (succ bool, result string, isSub bool) {
	if pat == "" {
		return false, "", false
	}

	reg, err := regexp.Compile(pat)
	if err != nil {
		return false, "", false
	}

	res := reg.FindStringSubmatch(log)
	switch len(res) {
	// 没查到
	case 0:
		return false, "", false
	// 没查到括号内的串，返回整个匹配串
	case 1:
		return true, res[0], false
	// 查到了，默认取第一个串
	default:
		var msg string
		if origin {
			msg = res[0]
			isSub = false
		} else {
			msg = res[1]
			isSub = true
		}
		return true, msg, isSub
	}
}

func includeIllegalChar(s string) bool {
	illegalChars := ":,=\r\n\t"
	return strings.ContainsAny(s, illegalChars)
}

// 生成返回错误信息
func genErrMsg(sign string) string {
	return fmt.Sprintf("正则匹配失败，请认真检查您[%s]的配置", sign)
}

// 生成子串匹配错误信息
func genSubErrMsg(sign string) string {
	return fmt.Sprintf("正则匹配成功。但根据配置，并没有获取到()内的子串，请认真检查您[%s]的配置", sign)
}

// 生成子串匹配错误信息
func genIllegalCharErrMsg() string {
	return fmt.Sprintf(`正则匹配成功。但是tag的key或者value包含非法字符:[:,/=\r\n\t], 请重新调整`)
}
