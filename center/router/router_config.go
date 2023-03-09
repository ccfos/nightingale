package router

import (
	"encoding/json"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) notifyChannelsGets(c *gin.Context) {
	var labelAndKeys []models.LabelAndKey
	cval, err := models.ConfigsGet(rt.Ctx, models.NOTIFYCHANNEL)
	ginx.Dangerous(err)

	if cval == "" {
		ginx.NewRender(c).Data(labelAndKeys, nil)
		return
	}

	var notifyChannels []models.NotifyChannel
	err = json.Unmarshal([]byte(cval), &notifyChannels)
	ginx.Dangerous(err)

	for _, v := range notifyChannels {
		if v.Hide {
			continue
		}
		var labelAndKey models.LabelAndKey
		labelAndKey.Label = v.Name
		labelAndKey.Key = v.Ident
		labelAndKeys = append(labelAndKeys, labelAndKey)
	}

	ginx.NewRender(c).Data(labelAndKeys, nil)
}

func (rt *Router) contactKeysGets(c *gin.Context) {
	var labelAndKeys []models.LabelAndKey
	cval, err := models.ConfigsGet(rt.Ctx, models.NOTIFYCONTACT)
	ginx.Dangerous(err)

	if cval == "" {
		ginx.NewRender(c).Data(labelAndKeys, nil)
		return
	}

	var notifyContacts []models.NotifyContact
	err = json.Unmarshal([]byte(cval), &notifyContacts)
	ginx.Dangerous(err)

	for _, v := range notifyContacts {
		if v.Hide {
			continue
		}
		var labelAndKey models.LabelAndKey
		labelAndKey.Label = v.Name
		labelAndKey.Key = v.Ident
		labelAndKeys = append(labelAndKeys, labelAndKey)
	}

	ginx.NewRender(c).Data(labelAndKeys, nil)
}
