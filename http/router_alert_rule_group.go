package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
)

func alertRuleGroupGets(c *gin.Context) {
	limit := queryInt(c, "limit", defaultLimit)
	query := queryStr(c, "query", "")

	total, err := models.AlertRuleGroupTotal(query)
	dangerous(err)

	list, err := models.AlertRuleGroupGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func alertRuleGroupFavoriteGet(c *gin.Context) {
	lst, err := loginUser(c).FavoriteAlertRuleGroups()
	renderData(c, lst, err)
}

type alertRuleGroupForm struct {
	Name         string `json:"name"`
	UserGroupIds string `json:"user_group_ids"`
}

func alertRuleGroupAdd(c *gin.Context) {
	var f alertRuleGroupForm
	bind(c, &f)

	me := loginUser(c).MustPerm("alert_rule_group_create")

	arg := models.AlertRuleGroup{
		Name:         f.Name,
		UserGroupIds: f.UserGroupIds,
		CreateBy:     me.Username,
		UpdateBy:     me.Username,
	}

	err := arg.Add()
	if err == nil {
		// 我创建的，顺便设置为我关注的
		models.AlertRuleGroupFavoriteAdd(arg.Id, me.Id)
	}

	renderMessage(c, err)
}

func alertRuleGroupGet(c *gin.Context) {
	alertRuleGroup := AlertRuleGroup(urlParamInt64(c, "id"))
	alertRuleGroup.FillUserGroups()
	renderData(c, alertRuleGroup, nil)
}

func alertRuleOfGroupGet(c *gin.Context) {
	ars, err := models.AlertRulesOfGroup(urlParamInt64(c, "id"))
	renderData(c, ars, err)
}

func alertRuleOfGroupDel(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	f.Validate()
	loginUser(c).MustPerm("alert_rule_delete")
	renderMessage(c, models.AlertRulesDel(f.Ids))
}

func alertRuleGroupPut(c *gin.Context) {
	var f alertRuleGroupForm
	bind(c, &f)

	me := loginUser(c).MustPerm("alert_rule_group_modify")
	arg := AlertRuleGroup(urlParamInt64(c, "id"))
	alertRuleWritePermCheck(arg, me)

	if arg.Name != f.Name {
		num, err := models.AlertRuleGroupCount("name=? and id<>?", f.Name, arg.Id)
		dangerous(err)

		if num > 0 {
			bomb(200, "AlertRuleGroup %s already exists", f.Name)
		}
	}

	arg.Name = f.Name
	arg.UserGroupIds = f.UserGroupIds
	arg.UpdateBy = me.Username
	arg.UpdateAt = time.Now().Unix()

	renderMessage(c, arg.Update("name", "update_by", "update_at", "user_group_ids"))
}

func alertRuleGroupDel(c *gin.Context) {
	me := loginUser(c).MustPerm("alert_rule_group_delete")
	arg := AlertRuleGroup(urlParamInt64(c, "id"))
	alertRuleWritePermCheck(arg, me)

	renderMessage(c, arg.Del())
}

func alertRuleGroupFavoriteAdd(c *gin.Context) {
	me := loginUser(c)
	arg := AlertRuleGroup(urlParamInt64(c, "id"))
	renderMessage(c, models.AlertRuleGroupFavoriteAdd(arg.Id, me.Id))
}

func alertRuleGroupFavoriteDel(c *gin.Context) {
	me := loginUser(c)
	arg := AlertRuleGroup(urlParamInt64(c, "id"))
	renderMessage(c, models.AlertRuleGroupFavoriteDel(arg.Id, me.Id))
}

func alertRuleWritePermCheck(alertRuleGroup *models.AlertRuleGroup, user *models.User) {
	if user.Role == "Admin" {
		return
	}

	gids := IdsInt64(alertRuleGroup.UserGroupIds)
	if len(gids) == 0 {
		// 压根没有配置管理团队，表示对所有Standard角色放开，那就不校验了
		return
	}

	for _, gid := range gids {
		if cache.UserGroupMember.Exists(gid, user.Id) {
			return
		}
	}

	bomb(http.StatusForbidden, "no permission")
}

func IdsInt64(ids string) []int64 {
	if ids == "" {
		return []int64{}
	}

	arr := strings.Fields(ids)
	count := len(arr)
	ret := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		if arr[i] != "" {
			id, err := strconv.ParseInt(arr[i], 10, 64)
			if err == nil {
				ret = append(ret, id)
			}
		}
	}

	return ret
}
