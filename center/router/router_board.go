package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/ginx"
)

type boardForm struct {
	Name    string `json:"name"`
	Ident   string `json:"ident"`
	Tags    string `json:"tags"`
	Configs string `json:"configs"`
	Public  int    `json:"public"`
}

func (rt *Router) boardAdd(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)

	board := &models.Board{
		GroupId:  ginx.UrlParamInt64(c, "id"),
		Name:     f.Name,
		Ident:    f.Ident,
		Tags:     f.Tags,
		Configs:  f.Configs,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	err := board.Add(rt.Ctx)
	ginx.Dangerous(err)

	if f.Configs != "" {
		ginx.Dangerous(models.BoardPayloadSave(rt.Ctx, board.Id, f.Configs))
	}

	ginx.NewRender(c).Data(board, nil)
}

func (rt *Router) boardGet(c *gin.Context) {
	bid := ginx.UrlParamStr(c, "bid")
	board, err := models.BoardGet(rt.Ctx, "id = ? or ident = ?", bid, bid)
	ginx.Dangerous(err)

	if board == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	if board.Public == 0 {
		rt.auth()(c)
		rt.user()(c)

		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			// check permission
			rt.bgroCheck(c, board.GroupId)
		}
	}

	ginx.NewRender(c).Data(board, nil)
}

func (rt *Router) boardPureGet(c *gin.Context) {
	board, err := models.BoardGetByID(rt.Ctx, ginx.UrlParamInt64(c, "bid"))
	ginx.Dangerous(err)

	if board == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	ginx.NewRender(c).Data(board, nil)
}

// bgrwCheck
func (rt *Router) boardDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	for i := 0; i < len(f.Ids); i++ {
		bid := f.Ids[i]

		board, err := models.BoardGet(rt.Ctx, "id = ?", bid)
		ginx.Dangerous(err)

		if board == nil {
			continue
		}

		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			// check permission
			rt.bgrwCheck(c, board.GroupId)
		}

		ginx.Dangerous(board.Del(rt.Ctx))
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) Board(id int64) *models.Board {
	obj, err := models.BoardGet(rt.Ctx, "id=?", id)
	ginx.Dangerous(err)

	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	return obj
}

// bgrwCheck
func (rt *Router) boardPut(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	bo := rt.Board(ginx.UrlParamInt64(c, "bid"))

	if !me.IsAdmin() {
		// check permission
		rt.bgrwCheck(c, bo.GroupId)
	}

	can, err := bo.CanRenameIdent(rt.Ctx, f.Ident)
	ginx.Dangerous(err)

	if !can {
		ginx.Bomb(http.StatusOK, "Ident duplicate")
	}

	bo.Name = f.Name
	bo.Ident = f.Ident
	bo.Tags = f.Tags
	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()

	err = bo.Update(rt.Ctx, "name", "ident", "tags", "update_by", "update_at")
	ginx.NewRender(c).Data(bo, err)
}

// bgrwCheck
func (rt *Router) boardPutConfigs(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)

	bid := ginx.UrlParamStr(c, "bid")
	bo, err := models.BoardGet(rt.Ctx, "id = ? or ident = ?", bid, bid)
	ginx.Dangerous(err)

	if bo == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	// check permission
	if !me.IsAdmin() {
		rt.bgrwCheck(c, bo.GroupId)
	}

	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()
	ginx.Dangerous(bo.Update(rt.Ctx, "update_by", "update_at"))

	bo.Configs = f.Configs
	ginx.Dangerous(models.BoardPayloadSave(rt.Ctx, bo.Id, f.Configs))

	ginx.NewRender(c).Data(bo, nil)
}

// bgrwCheck
func (rt *Router) boardPutPublic(c *gin.Context) {
	var f boardForm
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	bo := rt.Board(ginx.UrlParamInt64(c, "bid"))

	// check permission
	if !me.IsAdmin() {
		rt.bgrwCheck(c, bo.GroupId)
	}

	bo.Public = f.Public
	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()

	err := bo.Update(rt.Ctx, "public", "update_by", "update_at")
	ginx.NewRender(c).Data(bo, err)
}

func (rt *Router) boardGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	query := ginx.QueryStr(c, "query", "")

	boards, err := models.BoardGetsByGroupId(rt.Ctx, bgid, query)
	ginx.NewRender(c).Data(boards, err)
}

func (rt *Router) boardClone(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	bo := rt.Board(ginx.UrlParamInt64(c, "bid"))

	newBoard := &models.Board{
		Name:     bo.Name + " Copy",
		Tags:     bo.Tags,
		GroupId:  bo.GroupId,
		CreateBy: me.Username,
		UpdateBy: me.Username,
	}

	if bo.Ident != "" {
		newBoard.Ident = uuid.NewString()
	}

	ginx.Dangerous(newBoard.Add(rt.Ctx))

	// clone payload
	payload, err := models.BoardPayloadGet(rt.Ctx, bo.Id)
	ginx.Dangerous(err)

	if payload != "" {
		ginx.Dangerous(models.BoardPayloadSave(rt.Ctx, newBoard.Id, payload))
	}

	ginx.NewRender(c).Message(nil)
}
