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
		boardId, err := strconv.ParseInt(f.SourceId, 10, 64)
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
			rt.bgroCheck(c, board.GroupId)
		}

		if f.ExpireAt <= 0 {
			ginx.Bomb(http.StatusBadRequest, "expire time is required")
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
