package router

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

type Noti struct {
	SubId   int64  `json:"sub_id"`
	Channel string `json:"channel"`
	Result  []*struct {
		Target  string `json:"target"`
		Status  uint8  `json:"status"`
		Details string `json:"details"`
	} `json:"result"`
}

func (rt *Router) notificationRecordList(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	lst, err := models.NotificaitonRecordsGetByEventId(rt.Ctx, eid)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"noti_res": buildNotiListByChannelAndSubId(lst),
	}, nil)
}

func buildNotiListByChannelAndSubId(notiList []*models.NotificaitonRecord) []*Noti {
	indexMap := make(map[string]int)
	res := make([]*Noti, 0)
	for _, n := range notiList {
		key := fmt.Sprintf("%s_%d", n.Channel, n.SubId)
		noti := &Noti{}
		if idx, ok := indexMap[key]; ok {
			noti = res[idx]
		} else {
			noti.Channel = n.Channel
			noti.SubId = n.SubId
			indexMap[key] = len(res)
			res = append(res, noti)
		}
		noti.Result = append(noti.Result, &struct {
			Target  string `json:"target"`
			Status  uint8  `json:"status"`
			Details string `json:"details"`
		}{n.Target, n.Status, n.Details})
	}
	return res
}
