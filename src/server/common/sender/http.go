package sender

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common/poster"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/toolkits/pkg/logger"
	"time"
)

func ToBody(e *models.AlertCurEvent) *Body {

	return &Body{
		RuleName:       e.RuleName,
		TriggerTime:    e.TriggerTime,
		Tags:           e.Tags,
		TagsJSON:       e.TagsJSON,
		TagsMap:        e.TagsMap,
		NotifyUsersObj: e.NotifyUsersObj,
	}
}

type Body struct {
	RuleName       string            `json:"rule_name"`
	TriggerTime    int64             `json:"trigger_time"`
	Tags           string            `json:"-"`                         // for db
	TagsJSON       []string          `json:"tags" gorm:"-"`             // for fe
	TagsMap        map[string]string `json:"-" gorm:"-"`                // for internal usage
	NotifyUsersObj []*models.User    `json:"notify_users_obj" gorm:"-"` // for notify.py
	Content        string            `json:"content"`
}

func SendHttp(b Body) {

	postUrl := config.C.Alerting.Http.Url

	res, code, err := poster.PostJSON(postUrl, time.Second*5, b)
	if err != nil {
		logger.Errorf("http_sender: result=fail url=%s code=%d error=%v response=%s", postUrl, code, err, string(res))
	} else {
		logger.Infof("http_sender: result=succ url=%s code=%d response=%s", postUrl, code, string(res))
	}
}
