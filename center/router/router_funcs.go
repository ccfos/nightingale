package router

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ibex"
	"github.com/gin-gonic/gin"

	"github.com/toolkits/pkg/ginx"
)

const defaultLimit = 300

func queryDatasourceIds(c *gin.Context) []int64 {
	datasourceIds := ginx.QueryStr(c, "datasource_ids", "")
	datasourceIds = strings.ReplaceAll(datasourceIds, ",", " ")
	idsStr := strings.Fields(datasourceIds)
	ids := make([]int64, len(idsStr))
	for i, idStr := range idsStr {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		ids[i] = id
	}
	return ids
}

type idsForm struct {
	Ids []int64 `json:"ids"`
}

func (f idsForm) Verify() {
	if len(f.Ids) == 0 {
		ginx.Bomb(http.StatusBadRequest, "ids empty")
	}
}

func User(ctx *ctx.Context, id int64) *models.User {
	obj, err := models.UserGetById(ctx, id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such user")
	}

	return obj
}

func UserGroup(ctx *ctx.Context, id int64) *models.UserGroup {
	obj, err := models.UserGroupGetById(ctx, id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such UserGroup")
	}

	return obj
}

func BusiGroup(ctx *ctx.Context, id int64) *models.BusiGroup {
	obj, err := models.BusiGroupGetById(ctx, id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such BusiGroup")
	}

	return obj
}

func Dashboard(ctx *ctx.Context, id int64) *models.Dashboard {
	obj, err := models.DashboardGet(ctx, "id=?", id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	return obj
}

type DoneIdsReply struct {
	Err string `json:"err"`
	Dat struct {
		List []int64 `json:"list"`
	} `json:"dat"`
}

type TaskCreateReply struct {
	Err string `json:"err"`
	Dat int64  `json:"dat"` // task.id
}

// return task.id, error
func TaskCreate(v interface{}, ibexc aconf.Ibex) (int64, error) {
	var res TaskCreateReply
	err := ibex.New(
		ibexc.Address,
		ibexc.BasicAuthUser,
		ibexc.BasicAuthPass,
		ibexc.Timeout,
	).
		Path("/ibex/v1/tasks").
		In(v).
		Out(&res).
		POST()

	if err != nil {
		return 0, err
	}

	if res.Err != "" {
		return 0, fmt.Errorf("response.err: %v", res.Err)
	}

	return res.Dat, nil
}
