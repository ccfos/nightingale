package http

import (
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

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

func collectRulesAdd(c *gin.Context) {
	var forms []collectRuleForm
	bind(c, &forms)

	me := loginUser(c).MustPerm("collect_rule_create")

	for _, f := range forms {
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

		dangerous(cr.Add())
	}

	renderMessage(c, nil)
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

func regExpCheck(c *gin.Context) {
	param := make(map[string]string)
	dangerous(c.ShouldBind(&param))

	ret := &RegExpCheck{
		Success: true,
		Data:    make([]map[string]string, 0),
	}

	calcMethod := param["func"]
	if calcMethod == "" {
		tmp := map[string]string{"func": "is empty"}
		ret.Data = append(ret.Data, tmp)
		renderData(c, ret, nil)
		return
	}

	// 处理主正则
	if re, ok := param["re"]; !ok || re == "" {
		tmp := map[string]string{"re": "regex does not exist or is empty"}
		ret.Data = append(ret.Data, tmp)
		renderData(c, ret, nil)
		return
	}

	// 匹配主正则
	suc, reRes, isSub := checkRegex(param["re"], param["log"])
	if !suc {
		ret.Success = false
		reRes = genErrMsg(param["re"])
		ret.Data = append(ret.Data, map[string]string{"re": reRes})
		renderData(c, ret, nil)
		return
	}
	if calcMethod == "histogram" && !isSub {
		ret.Success = false
		reRes = genSubErrMsg(param["re"])
		ret.Data = append(ret.Data, map[string]string{"re": reRes})
		renderData(c, ret, nil)
		return
	}

	ret.Data = append(ret.Data, map[string]string{"re": reRes})
	// 处理tags
	var nonTagKey = map[string]bool{
		"re":   true,
		"log":  true,
		"func": true,
	}

	for tagk, pat := range param {
		// 如果不是tag，就继续循环
		if _, ok := nonTagKey[tagk]; ok {
			continue
		}
		suc, tagRes, isSub := checkRegex(pat, param["log"])
		if !suc {
			// 正则错误
			ret.Success = false
			tagRes = genErrMsg(pat)
		} else if !isSub {
			// 未匹配出子串
			ret.Success = false
			tagRes = genSubErrMsg(pat)
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

// 出错信息直接放在body里
func checkRegex(pat string, log string) (succ bool, result string, isSub bool) {
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
		return true, res[1], true
	}
}

func includeIllegalChar(s string) bool {
	illegalChars := ":,=\r\n\t"
	return strings.ContainsAny(s, illegalChars)
}

// 生成返回错误信息
func genErrMsg(pattern string) string {
	return _s("Regexp[%s] matching failed", pattern)
}

// 生成子串匹配错误信息
func genSubErrMsg(pattern string) string {
	return _s("Regexp[%s] matched, but cannot get substring()", pattern)
}

// 生成子串匹配错误信息
func genIllegalCharErrMsg() string {
	return _s(`TagKey or TagValue contains illegal characters[:,/=\r\n\t]`)
}
