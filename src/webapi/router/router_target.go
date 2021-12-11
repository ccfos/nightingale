package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func targetGets(c *gin.Context) {
	bgid := ginx.QueryInt64(c, "bgid", -1)
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 30)
	clusters := queryClusters(c)

	total, err := models.TargetTotal(bgid, clusters, query)
	ginx.Dangerous(err)

	list, err := models.TargetGets(bgid, clusters, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	if err == nil {
		cache := make(map[int64]*models.BusiGroup)
		for i := 0; i < len(list); i++ {
			ginx.Dangerous(list[i].FillGroup(cache))
		}
	}

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func targetGetTags(c *gin.Context) {
	idents := ginx.QueryStr(c, "idents")
	idents = strings.ReplaceAll(idents, ",", " ")
	lst, err := models.TargetGetTags(strings.Fields(idents))
	ginx.NewRender(c).Data(lst, err)
}

type targetTagsForm struct {
	Idents []string `json:"idents" binding:"required"`
	Tags   []string `json:"tags" binding:"required"`
}

func targetBindTags(c *gin.Context) {
	var f targetTagsForm
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents empty")
	}

	checkTargetPerm(c, f.Idents)

	// verify
	for i := 0; i < len(f.Tags); i++ {
		arr := strings.Split(f.Tags[i], "=")
		if len(arr) != 2 {
			ginx.Bomb(200, "invalid tag(%s)", f.Tags[i])
		}

		if strings.TrimSpace(arr[0]) == "" || strings.TrimSpace(arr[1]) == "" {
			ginx.Bomb(200, "invalid tag(%s)", f.Tags[i])
		}

		if strings.IndexByte(arr[0], '.') != -1 {
			ginx.Bomb(200, "invalid tagkey(%s): cannot contains .", arr[0])
		}

		if strings.IndexByte(arr[0], '-') != -1 {
			ginx.Bomb(200, "invalid tagkey(%s): cannot contains -", arr[0])
		}

		if !model.LabelNameRE.MatchString(arr[0]) {
			ginx.Bomb(200, "invalid tagkey(%s)", arr[0])
		}
	}

	for i := 0; i < len(f.Idents); i++ {
		target, err := models.TargetGetByIdent(f.Idents[i])
		ginx.Dangerous(err)

		if target == nil {
			continue
		}

		// 不能有同key的标签，否则附到时序数据上会产生覆盖，让人困惑
		for j := 0; j < len(f.Tags); j++ {
			tagkey := strings.Split(f.Tags[j], "=")[0]
			tagkeyPrefix := tagkey + "="
			if strings.HasPrefix(target.Tags, tagkeyPrefix) {
				ginx.NewRender(c).Message("duplicate tagkey(%s)", tagkey)
				return
			}
		}

		ginx.Dangerous(target.AddTags(f.Tags))
	}

	ginx.NewRender(c).Message(nil)
}

func targetUnbindTags(c *gin.Context) {
	var f targetTagsForm
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents empty")
	}

	checkTargetPerm(c, f.Idents)

	for i := 0; i < len(f.Idents); i++ {
		target, err := models.TargetGetByIdent(f.Idents[i])
		ginx.Dangerous(err)

		if target == nil {
			continue
		}

		ginx.Dangerous(target.DelTags(f.Tags))
	}

	ginx.NewRender(c).Message(nil)
}

type targetNoteForm struct {
	Idents []string `json:"idents" binding:"required"`
	Note   string   `json:"note"`
}

func targetUpdateNote(c *gin.Context) {
	var f targetNoteForm
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents empty")
	}

	checkTargetPerm(c, f.Idents)

	ginx.NewRender(c).Message(models.TargetUpdateNote(f.Idents, f.Note))
}

type targetBgidForm struct {
	Idents []string `json:"idents" binding:"required"`
	Bgid   int64    `json:"bgid"`
}

func targetUpdateBgid(c *gin.Context) {
	var f targetBgidForm
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents empty")
	}

	user := c.MustGet("user").(*models.User)
	if user.IsAdmin() {
		ginx.NewRender(c).Message(models.TargetUpdateBgid(f.Idents, f.Bgid, false))
		return
	}

	if f.Bgid > 0 {
		// 把要操作的机器分成两部分，一部分是bgid为0，需要管理员分配，另一部分bgid>0，说明是业务组内部想调整
		// 比如原来分配给didiyun的机器，didiyun的管理员想把部分机器调整到didiyun-ceph下
		// 对于调整的这种情况，当前登录用户要对这批机器有操作权限，同时还要对目标BG有操作权限
		orphans, err := models.IdentsFilter(f.Idents, "group_id = ?", 0)
		ginx.Dangerous(err)

		// 机器里边存在未归组的，登录用户就需要是admin
		if len(orphans) > 0 && !user.IsAdmin() {
			ginx.Bomb(http.StatusForbidden, "No permission. Only admin can assign BG")
		}

		reBelongs, err := models.IdentsFilter(f.Idents, "group_id > ?", 0)
		ginx.Dangerous(err)

		if len(reBelongs) > 0 {
			// 对于这些要重新分配的机器，操作者要对这些机器本身有权限，同时要对目标bgid有权限
			checkTargetPerm(c, f.Idents)

			bg := BusiGroup(f.Bgid)
			can, err := user.CanDoBusiGroup(bg, "rw")
			ginx.Dangerous(err)

			if !can {
				ginx.Bomb(http.StatusForbidden, "No permission. You are not admin of BG(%s)", bg.Name)
			}
		}
	} else if f.Bgid == 0 {
		// 退还机器
		checkTargetPerm(c, f.Idents)
	} else {
		ginx.Bomb(http.StatusBadRequest, "invalid bgid")
	}

	ginx.NewRender(c).Message(models.TargetUpdateBgid(f.Idents, f.Bgid, false))
}

type identsForm struct {
	Idents []string `json:"idents" binding:"required"`
}

func targetDel(c *gin.Context) {
	var f identsForm
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents empty")
	}

	checkTargetPerm(c, f.Idents)

	ginx.NewRender(c).Message(models.TargetDel(f.Idents))
}

func checkTargetPerm(c *gin.Context, idents []string) {
	user := c.MustGet("user").(*models.User)
	nopri, err := user.NopriIdents(idents)
	ginx.Dangerous(err)

	if len(nopri) > 0 {
		ginx.Bomb(http.StatusForbidden, "No permission to operate the targets: %s", strings.Join(nopri, ", "))
	}
}
