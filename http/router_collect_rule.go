package http

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
)

type collectRuleForm struct {
	ClasspathId int64  `json:"classpath_id"`
	PrefixMatch int    `json:"prefix_match"`
	Name        string `json:"name"`
	Note        string `json:"note"`
	Step        int    `json:"step"`
	Type        string `json:"type"`
	Data        string `json:"data"`
	AppendTags  string `json:"append_tags"`
}

func collectRuleAdd(c *gin.Context) {
	var f collectRuleForm
	bind(c, &f)

	me := loginUser(c).MustPerm("collect_rule_create")

	cr := models.CollectRule{
		ClasspathId: f.ClasspathId,
		PrefixMatch: f.PrefixMatch,
		Name:        f.Name,
		Note:        f.Note,
		Step:        f.Step,
		Type:        f.Type,
		Data:        f.Data,
		AppendTags:  f.AppendTags,
		CreateBy:    me.Username,
		UpdateBy:    me.Username,
	}

	renderMessage(c, cr.Add())
}

func collectRulePut(c *gin.Context) {
	var f collectRuleForm
	bind(c, &f)

	me := loginUser(c).MustPerm("collect_rule_modify")
	cr := CollectRule(urlParamInt64(c, "id"))

	cr.PrefixMatch = f.PrefixMatch
	cr.Name = f.Name
	cr.Note = f.Note
	cr.Step = f.Step
	cr.Type = f.Type
	cr.Data = f.Data
	cr.AppendTags = f.AppendTags
	cr.UpdateAt = time.Now().Unix()
	cr.UpdateBy = me.Username

	renderMessage(c, cr.Update(
		"prefix_match",
		"name",
		"note",
		"step",
		"type",
		"data",
		"update_at",
		"update_by",
		"append_tags",
	))
}

func collectRuleDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()
	loginUser(c).MustPerm("collect_rule_delete")
	renderMessage(c, models.CollectRulesDel(f.Ids))
}

func collectRuleGets(c *gin.Context) {
	classpathId := urlParamInt64(c, "id")

	where := "classpath_id = ?"
	param := []interface{}{classpathId}

	typ := queryStr(c, "type", "")
	if typ != "" {
		where += " and type = ?"
		param = append(param, typ)
	}

	objs, err := models.CollectRuleGets(where, param...)
	renderData(c, objs, err)
}

func collectRuleGetsByIdent(c *gin.Context) {
	ident := queryStr(c, "ident")

	objs := cache.CollectRulesOfIdent.GetBy(ident)
	renderData(c, objs, nil)
}

type Summary struct {
	LatestUpdatedAt int64 `json:"latestUpdatedAt"`
	Total           int   `json:"total"`
}

func collectRuleSummaryGetByIdent(c *gin.Context) {
	ident := queryStr(c, "ident")
	var summary Summary
	objs := cache.CollectRulesOfIdent.GetBy(ident)
	total := len(objs)
	if total > 0 {
		summary.Total = total
		var latestUpdatedAt int64
		for _, obj := range objs {
			if latestUpdatedAt < obj.UpdateAt {
				latestUpdatedAt = obj.UpdateAt
			}
		}
		summary.LatestUpdatedAt = latestUpdatedAt
	}

	renderData(c, summary, nil)
}

type RegExpCheck struct {
	Success bool                `json:"success"`
	Data    []map[string]string `json:"tags"`
}

var RegExpExcludePatition string = "```EXCLUDE```"

func regExpCheck(c *gin.Context) {
	param := make(map[string]string)
	dangerous(c.ShouldBind(&param))

	ret := &RegExpCheck{
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
	calc_method := param["calc_method"]

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
	case "dd mmm yyyy HH:MM:SS":
		pat = `([012][0-9]|3[01])\s+[JFMASOND][a-z]{2}\s+(2[0-9]{3})\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02 Jan 2006 15:04:05"
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
	return `正则匹配成功。但是tag的key或者value包含非法字符:[:,/=\r\n\t], 请重新调整`
}
