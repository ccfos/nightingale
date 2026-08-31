package router

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

// maxBoardTokenTTL 仪表盘分享令牌的有效期上限：99 年。
//
// 上限的意义不在于「99 年够不够用」，而在于堵住「事实上永不过期」——前端有效期
// 输入框原先没有上限，填 999999999 年会算出 3e16 量级的 ExpireAt，IsExpired 永远
// 为假，等于绕开了下面那条「不允许永不过期」的约束（顺带还会让前端 moment 把过期
// 时间渲染成 Invalid date）。
//
// 与前端同口径按 365 天/年折算，另留一天容差：ExpireAt 由前端用自己的时钟算出，
// 客户端时钟偏快时差值会略微超过上限，不该因此误拒。
const maxBoardTokenTTL int64 = 99*365*24*3600 + 24*3600

// sourceTokenAdd 生成新的源令牌
func (rt *Router) sourceTokenAdd(c *gin.Context) {
	var f models.SourceToken
	ginx.BindJSON(c, &f)

	// source_type 归一化 + 白名单：消费端用 SQL `source_type = 'board'` 匹配，而
	// source_token 表无 COLLATE、MySQL 默认排序规则大小写不敏感，若此处放行 `Board`
	// 之类变体，签发侧的大小写敏感分支会被跳过、消费侧却照常命中，造成越权签发。
	// 统一小写去空白后再校验，签发判定与 SQL 匹配指向同一个值。
	f.SourceType = strings.ToLower(strings.TrimSpace(f.SourceType))
	if f.SourceType != models.SourceTypeEvent && f.SourceType != models.SourceTypeBoard {
		ginx.Bomb(http.StatusBadRequest, "invalid source_type")
	}

	if f.ExpireAt > 0 && f.ExpireAt <= time.Now().Unix() {
		ginx.Bomb(http.StatusBadRequest, "expire time must be in the future")
	}

	// 仪表盘分享令牌：校验资源存在与签发者权限，且必须限时——
	// token 会把板内数据源的只读查询能力开放给链接持有者，不允许永不过期
	if f.SourceType == models.SourceTypeBoard {
		boardId := rt.checkBoardTokenOwner(c, f.SourceId)

		if f.ExpireAt <= 0 {
			ginx.Bomb(http.StatusBadRequest, "expire time is required")
		}

		if f.ExpireAt-time.Now().Unix() > maxBoardTokenTTL {
			ginx.Bomb(http.StatusBadRequest, "expire time is too far in the future")
		}

		// 备注必填：一个板可能同时存在多条长期有效的分享链接，没有备注就无从
		// 判断哪条该注销。前端已做必填校验，这里是接口层的兜底
		f.Note = strings.TrimSpace(f.Note)
		if f.Note == "" {
			ginx.Bomb(http.StatusBadRequest, "note is required")
		}

		// 规范化回解析结果，避免 `05`、`5 ` 这类写入形态与 boardGet 读取时
		// fmt.Sprintf("%d", board.Id) 的形态对不上
		f.SourceId = strconv.FormatInt(boardId, 10)
	}

	token := uuid.New().String()

	username := c.MustGet("username").(string)

	f.Token = token
	f.CreateBy = username
	f.CreateAt = time.Now().Unix()

	err := f.Add(rt.Ctx)
	ginx.Dangerous(err)

	go models.CleanupExpiredTokens(rt.Ctx)
	ginx.NewRender(c).Data(token, nil)
}

// checkBoardTokenOwner 校验调用者对该仪表盘有权限（签发/查看/注销分享令牌共用），
// 返回规范化后的 board id。
//
// 用 bgrwCheck 而非 bgroCheck：bgroCheck 走的是不带 permFlag 的 CanDoBusiGroup，
// 业务组的只读成员也会通过。而这三个接口的能力比「把看板设为 public」更敏感——
// 签发一条令牌等于把看板配置连同板内全部数据源的匿名查询能力交给任意第三方，
// 且链接在过期前持续有效（即便签发者之后被移出业务组）。对齐同等能力的
// PUT /board/:bid/public：它要求 perm("/dashboards/put") + bgrwCheck。
//
// 列表接口同样按写权限判定，不给只读成员开口子：SourceTokenGets 的响应里带
// token 原文，能列举就等于能拿到一条可用的匿名链接转发出去，与签发无实质差别。
func (rt *Router) checkBoardTokenOwner(c *gin.Context, sourceId string) int64 {
	boardId, err := strconv.ParseInt(sourceId, 10, 64)
	if err != nil || boardId <= 0 {
		ginx.Bomb(http.StatusBadRequest, "invalid source_id")
	}

	board, err := models.BoardGetByID(rt.Ctx, boardId)
	ginx.Dangerous(err)
	if board == nil {
		ginx.Bomb(http.StatusNotFound, "No such dashboard")
	}

	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		// 权限点在 handler 内判定而不是挂到路由上：POST /source-token 是 board 与
		// event 两种令牌共用的入口，挂 rt.perm("/dashboards/put") 会让原有的事件
		// 分享也要求仪表盘写权限。这里只对 board 分支生效，event 流程不受影响。
		can, err := me.CheckPerm(rt.Ctx, "/dashboards/put")
		ginx.Dangerous(err)
		if !can {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}

		rt.bgrwCheck(c, board.GroupId)
	}

	return boardId
}

// sourceTokenGets 列出某个资源已签发的分享令牌，供页面展示与注销
func (rt *Router) sourceTokenGets(c *gin.Context) {
	sourceType := strings.ToLower(strings.TrimSpace(ginx.QueryStr(c, "source_type", "")))
	sourceId := ginx.QueryStr(c, "source_id", "")

	if sourceType != models.SourceTypeBoard {
		// 目前只有仪表盘分享需要管理界面；其余类型不开放列举，避免泄露令牌
		ginx.Bomb(http.StatusBadRequest, "invalid source_type")
	}

	boardId := rt.checkBoardTokenOwner(c, sourceId)

	lst, err := models.SourceTokenGets(rt.Ctx, sourceType, strconv.FormatInt(boardId, 10))
	ginx.NewRender(c).Data(lst, err)
}

// sourceTokenDel 注销分享令牌，链接立即失效
func (rt *Router) sourceTokenDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	st, err := models.SourceTokenGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if st == nil {
		ginx.NewRender(c).Message(nil)
		return
	}

	if st.SourceType != models.SourceTypeBoard {
		ginx.Bomb(http.StatusBadRequest, "invalid source_type")
	}
	rt.checkBoardTokenOwner(c, st.SourceId)

	ginx.NewRender(c).Message(models.SourceTokenDel(rt.Ctx, id))
}
