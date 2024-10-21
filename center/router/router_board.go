package router

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/str"
)

type boardForm struct {
	Name       string  `json:"name"`
	Ident      string  `json:"ident"`
	Tags       string  `json:"tags"`
	Configs    string  `json:"configs"`
	Public     int     `json:"public"`
	PublicCate int     `json:"public_cate"`
	Bgids      []int64 `json:"bgids"`
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

	if board.PublicCate == models.PublicLogin {
		rt.auth()(c)
	} else if board.PublicCate == models.PublicBusi {
		rt.auth()(c)
		rt.user()(c)

		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			bgids, err := models.MyBusiGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)
			if len(bgids) == 0 {
				ginx.Bomb(http.StatusForbidden, "forbidden")
			}

			ok, err := models.BoardBusigroupCheck(rt.Ctx, board.Id, bgids)
			ginx.Dangerous(err)
			if !ok {
				ginx.Bomb(http.StatusForbidden, "forbidden")
			}
		}
	}

	ginx.NewRender(c).Data(board, nil)
}

// 根据 bids 参数，获取多个 board
func (rt *Router) boardGetsByBids(c *gin.Context) {
	bids := str.IdsInt64(ginx.QueryStr(c, "bids", ""), ",")
	boards, err := models.BoardGetsByBids(rt.Ctx, bids)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(boards, err)
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
	bo.PublicCate = f.PublicCate

	if bo.PublicCate == models.PublicBusi {
		err := models.BoardBusigroupUpdate(rt.Ctx, bo.Id, f.Bgids)
		ginx.Dangerous(err)
	} else {
		err := models.BoardBusigroupDelByBoardId(rt.Ctx, bo.Id)
		ginx.Dangerous(err)
	}

	bo.UpdateBy = me.Username
	bo.UpdateAt = time.Now().Unix()

	err := bo.Update(rt.Ctx, "public", "public_cate", "update_by", "update_at")
	ginx.NewRender(c).Data(bo, err)
}

func (rt *Router) boardGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	query := ginx.QueryStr(c, "query", "")

	boards, err := models.BoardGetsByGroupId(rt.Ctx, bgid, query)
	ginx.NewRender(c).Data(boards, err)
}

func (rt *Router) publicBoardGets(c *gin.Context) {
	me := c.MustGet("user").(*models.User)

	bgids, err := models.MyBusiGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	boardIds, err := models.BoardIdsByBusiGroupIds(rt.Ctx, bgids)
	ginx.Dangerous(err)

	boards, err := models.BoardGets(rt.Ctx, "", "public=1 and (public_cate in (?) or id in (?))", []int64{0, 1}, boardIds)
	ginx.NewRender(c).Data(boards, err)
}

func (rt *Router) boardGetsByGids(c *gin.Context) {
	gids := str.IdsInt64(ginx.QueryStr(c, "gids", ""), ",")
	query := ginx.QueryStr(c, "query", "")

	if len(gids) > 0 {
		for _, gid := range gids {
			rt.bgroCheck(c, gid)
		}
	} else {
		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			var err error
			gids, err = models.MyBusiGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)

			if len(gids) == 0 {
				ginx.NewRender(c).Data([]int{}, nil)
				return
			}
		}
	}

	boardBusigroups, err := models.BoardBusigroupGets(rt.Ctx)
	ginx.Dangerous(err)
	m := make(map[int64][]int64)
	for _, boardBusigroup := range boardBusigroups {
		m[boardBusigroup.BoardId] = append(m[boardBusigroup.BoardId], boardBusigroup.BusiGroupId)
	}

	boards, err := models.BoardGetsByBGIds(rt.Ctx, gids, query)
	ginx.Dangerous(err)
	for i := 0; i < len(boards); i++ {
		if ids, ok := m[boards[i].Id]; ok {
			boards[i].Bgids = ids
		}
	}

	ginx.NewRender(c).Data(boards, err)
}

func (rt *Router) boardClone(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	bo := rt.Board(ginx.UrlParamInt64(c, "bid"))

	newBoard := bo.Clone(me.Username, bo.GroupId, " Cloned")

	ginx.Dangerous(newBoard.Add(rt.Ctx))

	// clone payload
	payload, err := models.BoardPayloadGet(rt.Ctx, bo.Id)
	ginx.Dangerous(err)

	if payload != "" {
		ginx.Dangerous(models.BoardPayloadSave(rt.Ctx, newBoard.Id, payload))
	}

	ginx.NewRender(c).Message(nil)
}

type boardsForm struct {
	BoardIds []int64 `json:"board_ids"`
	Bgids    []int64 `json:"bgids"`
}

func (rt *Router) boardBatchClone(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	var f boardsForm
	ginx.BindJSON(c, &f)

	for _, bgid := range f.Bgids {
		rt.bgrwCheck(c, bgid)
	}

	reterr := make(map[string]string, len(f.BoardIds))
	lang := c.GetHeader("X-Language")

	for _, bgid := range f.Bgids {
		for _, bid := range f.BoardIds {
			bo := rt.Board(bid)
			newBoard := bo.Clone(me.Username, bgid, "")
			payload, err := models.BoardPayloadGet(rt.Ctx, bo.Id)
			if err != nil {
				reterr[fmt.Sprintf("%s-%d", newBoard.Name, bgid)] = i18n.Sprintf(lang, err.Error())
				continue
			}

			if err = newBoard.AtomicAdd(rt.Ctx, payload); err != nil {
				reterr[fmt.Sprintf("%s-%d", newBoard.Name, bgid)] = i18n.Sprintf(lang, err.Error())
			}
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}
