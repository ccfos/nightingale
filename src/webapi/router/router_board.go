package router

import (
	"net/http"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

type boardForm struct {
	Name    string `json:"name"`
	Tags    string `json:"tags"`
	Configs string `json:"configs"`
	Public  int    `json:"public"`
}

func boardAdd(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)

	board := &models.Board{
		GroupId:  ginx.UrlParamInt64(c, "id"),
		Name:     f.Name,
		Tags:     f.Tags,
		Configs:  f.Configs,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	err := board.Add()
	ginx.Dangerous(err)

	if f.Configs != "" {
		ginx.Dangerous(models.BoardPayloadSave(board.Id, f.Configs))
	}

	ginx.NewRender(c).Data(board, nil)
}

func boardGet(c *gin.Context) {
	board, err := models.BoardGet("id = ?", ginx.UrlParamInt64(c, "bid"))
	ginx.Dangerous(err)

	if board == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	if board.Public == 0 {
		auth()(c)
		user()(c)

		bgroCheck(c, board.GroupId)
	}

	ginx.NewRender(c).Data(board, nil)
}

func boardPureGet(c *gin.Context) {
	board, err := models.BoardGetByID(ginx.UrlParamInt64(c, "bid"))
	ginx.Dangerous(err)

	if board == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	ginx.NewRender(c).Data(board, nil)
}

// bgrwCheck
func boardDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	for i := 0; i < len(f.Ids); i++ {
		bid := f.Ids[i]

		board, err := models.BoardGet("id = ?", bid)
		ginx.Dangerous(err)

		if board == nil {
			continue
		}

		// check permission
		bgrwCheck(c, board.GroupId)

		ginx.Dangerous(board.Del())
	}

	ginx.NewRender(c).Message(nil)
}

func Board(id int64) *models.Board {
	obj, err := models.BoardGet("id=?", id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	return obj
}

// bgrwCheck
func boardPut(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	bo := Board(ginx.UrlParamInt64(c, "bid"))

	// check permission
	bgrwCheck(c, bo.GroupId)

	bo.Name = f.Name
	bo.Tags = f.Tags
	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()

	err := bo.Update("name", "tags", "update_by", "update_at")
	ginx.NewRender(c).Data(bo, err)
}

// bgrwCheck
func boardPutConfigs(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	bo := Board(ginx.UrlParamInt64(c, "bid"))

	// check permission
	bgrwCheck(c, bo.GroupId)

	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()
	ginx.Dangerous(bo.Update("update_by", "update_at"))

	bo.Configs = f.Configs
	ginx.Dangerous(models.BoardPayloadSave(bo.Id, f.Configs))

	ginx.NewRender(c).Data(bo, nil)
}

// bgrwCheck
func boardPutPublic(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	bo := Board(ginx.UrlParamInt64(c, "bid"))

	// check permission
	bgrwCheck(c, bo.GroupId)

	bo.Public = f.Public
	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()

	err := bo.Update("public", "update_by", "update_at")
	ginx.NewRender(c).Data(bo, err)
}

func boardGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	query := ginx.QueryStr(c, "query", "")

	boards, err := models.BoardGets(bgid, query)
	ginx.NewRender(c).Data(boards, err)
}

func boardClone(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	bo := Board(ginx.UrlParamInt64(c, "bid"))

	newBoard := &models.Board{
		Name:     bo.Name + " Copy",
		Tags:     bo.Tags,
		GroupId:  bo.GroupId,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	ginx.Dangerous(newBoard.Add())

	// clone payload
	payload, err := models.BoardPayloadGet(bo.Id)
	ginx.Dangerous(err)

	if payload != "" {
		ginx.Dangerous(models.BoardPayloadSave(newBoard.Id, payload))
	}

	ginx.NewRender(c).Message(nil)
}

// ---- migrate ----

func migrateDashboards(c *gin.Context) {
	lst, err := models.DashboardGetAll()
	ginx.NewRender(c).Data(lst, err)
}

func migrateDashboardGet(c *gin.Context) {
	dash := Dashboard(ginx.UrlParamInt64(c, "id"))
	ginx.NewRender(c).Data(dash, nil)
}

func migrateDashboard(c *gin.Context) {
	dash := Dashboard(ginx.UrlParamInt64(c, "id"))

	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)

	board := &models.Board{
		GroupId:  dash.GroupId,
		Name:     f.Name,
		Tags:     f.Tags,
		Configs:  f.Configs,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	ginx.Dangerous(board.Add())

	if board.Configs != "" {
		ginx.Dangerous(models.BoardPayloadSave(board.Id, board.Configs))
	}

	ginx.NewRender(c).Message(dash.Del())
}
