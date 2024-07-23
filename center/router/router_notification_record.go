package router

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
)

type Noti struct {
	SubId   int64         `json:"sub_id"`
	Channel string        `json:"channel"`
	Result  []*NotiDetail `json:"result"`
}

type NotiDetail struct {
	Target    string   `json:"target"`
	Status    uint8    `json:"status"`
	Details   []string `json:"details,omitempty"`
	Username  string   `json:"username,omitempty"`
	CreatedAt int64    `json:"created_at"`
}

func (rt *Router) notificationRecordList(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	lst, err := models.NotificaitonRecordsGetByEventId(rt.Ctx, eid)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"noti_res": buildNotiListByChannelAndSubId(rt.Ctx, lst),
	}, nil)
}

func buildNotiListByChannelAndSubId(ctx *ctx.Context, nl []*models.NotificaitonRecord) []*Noti {
	indexMap := make(map[string]int)
	usernameByTarget := make(map[string][]string)
	groupIdSet := make(map[int64]struct{})
	res := make([]*Noti, 0)
	for _, n := range nl {
		key := fmt.Sprintf("%s_%d", n.Channel, n.SubId)
		noti := &Noti{}
		if idx, ok := indexMap[key]; ok {
			noti = res[idx]
		} else {
			noti.Channel = n.Channel
			noti.SubId = n.SubId
			fillUserName(ctx, n, usernameByTarget, groupIdSet)

			indexMap[key] = len(res)
			res = append(res, noti)
		}

		if idx, ok := noti.MergeIdx(n.Target); !ok {
			noti.Result = append(noti.Result, &NotiDetail{n.Target,
				n.Status, []string{n.Details}, "", n.CreatedAt.Unix()})
		} else {
			noti.Result[idx].Details = append(noti.Result[idx].Details, n.Details)
			if n.Status == models.NotiStatusFailure {
				noti.Result[idx].Status = models.NotiStatusFailure
			}
		}
	}

	for _, ns := range res {
		for _, n := range ns.Result {
			n.Username = strings.Join(usernameByTarget[n.Target], ",")
		}
	}

	return res
}

func (n *Noti) MergeIdx(curTarget string) (int, bool) {
	if n == nil || len(n.Result) == 0 {
		return 0, false
	}
	for idx, nd := range n.Result {
		if nd.Target == curTarget {
			return idx, true
		}
	}
	return 0, false
}

func fillUserName(ctx *ctx.Context, noti *models.NotificaitonRecord,
	userNameByTarget map[string][]string, groupIdSet map[int64]struct{}) {
	if !slice.ContainsString(models.DefaultChannels, noti.Channel) {
		return
	}
	gids := make([]int64, 0)
	for _, gid := range noti.GetGroupIds(ctx) {
		if _, ok := groupIdSet[gid]; !ok {
			gids = append(gids, gid)
		} else {
			groupIdSet[gid] = struct{}{}
		}
	}

	users, err := models.UsersGetByGroupIds(ctx, gids)
	if err != nil {
		logger.Errorf("UsersGetByGroupIds failed, err: %v", err)
		return
	}
	for _, user := range users {
		for _, ch := range models.DefaultChannels {
			target, exist := user.ExtractToken(ch)
			usl := userNameByTarget[target]
			if exist && !slice.ContainsString(usl, user.Username) {
				userNameByTarget[target] = append(usl, user.Username)
			}
		}
	}
}
