package http

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/scache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type CollectRecv struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func collectRulePost(c *gin.Context) {
	var recv []CollectRecv
	errors.Dangerous(c.ShouldBind(&recv))

	creator := loginUsername(c)
	for _, obj := range recv {
		cl, err := collector.GetCollector(obj.Type)
		errors.Dangerous(err)

		if err := cl.Create([]byte(obj.Data), creator); err != nil {
			errors.Bomb("%s add rule err %s", obj.Type, err)
		}
	}

	renderData(c, "ok", nil)
}

func collectRulesGetByLocalEndpoint(c *gin.Context) {
	collect := scache.CollectCache.GetBy(urlParamStr(c, "endpoint"))
	renderData(c, collect, nil)
}

func collectRuleGet(c *gin.Context) {
	t := mustQueryStr(c, "type")
	id := mustQueryInt64(c, "id")

	cl, err := collector.GetCollector(t)
	errors.Dangerous(err)

	ret, err := cl.Get(id)
	renderData(c, ret, err)
}

func collectRulesGet(c *gin.Context) {
	nid := queryInt64(c, "nid", -1)
	tp := queryStr(c, "type", "")
	var resp []interface{}
	var types []string

	if tp == "" {
		types = []string{"port", "proc", "log", "plugin"}
	} else {
		types = []string{tp}
	}

	nids := []int64{nid}
	for _, t := range types {
		cl, err := collector.GetCollector(t)
		if err != nil {
			logger.Warning(t, err)
			continue
		}

		ret, err := cl.Gets(nids)
		if err != nil {
			logger.Warning(t, err)
			continue
		}
		resp = append(resp, ret...)
	}

	renderData(c, resp, nil)
}

func collectRulesGetV2(c *gin.Context) {
	nid := queryInt64(c, "nid", 0)
	limit := queryInt(c, "limit", 20)
	typ := queryStr(c, "type", "")

	total, list, err := models.GetCollectRules(typ, nid, limit, offset(c, limit, 0))

	renderData(c, map[string]interface{}{
		"total": total,
		"list":  list,
	}, err)
}

func collectRulePut(c *gin.Context) {
	var recv CollectRecv
	errors.Dangerous(c.ShouldBind(&recv))

	cl, err := collector.GetCollector(recv.Type)
	errors.Dangerous(err)

	creator := loginUsername(c)
	if err := cl.Update([]byte(recv.Data), creator); err != nil {
		errors.Bomb("%s update rule err %s", recv.Type, err)
	}
	renderData(c, "ok", nil)
}

type CollectsDelRev struct {
	Type string  `json:"type"`
	Ids  []int64 `json:"ids"`
}

func collectsRuleDel(c *gin.Context) {
	var recv []CollectsDelRev
	errors.Dangerous(c.ShouldBind(&recv))

	username := loginUsername(c)
	for _, obj := range recv {
		for i := 0; i < len(obj.Ids); i++ {
			cl, err := collector.GetCollector(obj.Type)
			errors.Dangerous(err)

			if err := cl.Delete(obj.Ids[i], username); err != nil {
				errors.Dangerous(err)
			}
		}
	}

	renderData(c, "ok", nil)
}

func collectRuleTypesGet(c *gin.Context) {
	category := mustQueryStr(c, "category")
	switch category {
	case "remote":
		renderData(c, collector.GetRemoteCollectors(), nil)
	case "local":
		renderData(c, collector.GetLocalCollectors(), nil)
	default:
		renderData(c, nil, nil)
	}
}

func collectRuleTemplateGet(c *gin.Context) {
	t := urlParamStr(c, "type")
	collector, err := collector.GetCollector(t)
	errors.Dangerous(err)

	tpl, err := collector.Template()
	renderData(c, tpl, err)
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

func collectRulesGetByRemoteEndpoint(c *gin.Context) {
	rules := scache.CollectRuleCache.GetBy(urlParamStr(c, "endpoint"))
	renderData(c, rules, nil)

}
