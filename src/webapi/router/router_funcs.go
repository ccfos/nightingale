package router

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/ibex"
	"github.com/didi/nightingale/v5/src/webapi/config"
)

const defaultLimit = 300

func queryClusters(c *gin.Context) []string {
	clusters := ginx.QueryStr(c, "clusters", "")
	clusters = strings.ReplaceAll(clusters, ",", " ")
	return strings.Fields(clusters)
}

func Cluster(c *gin.Context) string {
	return c.GetHeader("X-Cluster")
}

func MustGetCluster(c *gin.Context) string {
	cluster := Cluster(c)
	if cluster == "" {
		ginx.Bomb(http.StatusBadRequest, "Header(X-Cluster) missed")
	}
	return cluster
}

type idsForm struct {
	Ids []int64 `json:"ids"`
}

func (f idsForm) Verify() {
	if len(f.Ids) == 0 {
		ginx.Bomb(http.StatusBadRequest, "ids empty")
	}
}

func User(id int64) *models.User {
	obj, err := models.UserGetById(id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such user")
	}

	return obj
}

func UserGroup(id int64) *models.UserGroup {
	obj, err := models.UserGroupGetById(id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such UserGroup")
	}

	return obj
}

func BusiGroup(id int64) *models.BusiGroup {
	obj, err := models.BusiGroupGetById(id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such BusiGroup")
	}

	return obj
}

func Dashboard(id int64) *models.Dashboard {
	obj, err := models.DashboardGet("id=?", id)
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

func TaskDoneIds(ids []int64) ([]int64, error) {
	var res DoneIdsReply
	err := ibex.New(
		config.C.Ibex.Address,
		config.C.Ibex.BasicAuthUser,
		config.C.Ibex.BasicAuthPass,
		config.C.Ibex.Timeout,
	).
		Path("/ibex/v1/tasks/done-ids").
		QueryString("ids", str.IdsString(ids, ",")).
		Out(&res).
		GET()

	if err != nil {
		return nil, err
	}

	if res.Err != "" {
		return nil, fmt.Errorf("response.err: %v", res.Err)
	}

	return res.Dat.List, nil
}

type TaskCreateReply struct {
	Err string `json:"err"`
	Dat int64  `json:"dat"` // task.id
}

// return task.id, error
func TaskCreate(v interface{}) (int64, error) {
	var res TaskCreateReply
	err := ibex.New(
		config.C.Ibex.Address,
		config.C.Ibex.BasicAuthUser,
		config.C.Ibex.BasicAuthPass,
		config.C.Ibex.Timeout,
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
