package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

func (rt *Router) aiConversationGets(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	lst, err := models.AIConversationGetsByUserId(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) aiConversationAdd(c *gin.Context) {
	var obj models.AIConversation
	ginx.BindJSON(c, &obj)

	me := c.MustGet("user").(*models.User)
	obj.UserId = me.Id
	ginx.Dangerous(obj.Verify())
	ginx.Dangerous(obj.Create(rt.Ctx))
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiConversationGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIConversationGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "conversation not found")
	}

	me := c.MustGet("user").(*models.User)
	if obj.UserId != me.Id {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}

	messages, err := models.AIConversationMessageGetsByConversationId(rt.Ctx, id)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"conversation": obj,
		"messages":     messages,
	}, nil)
}

func (rt *Router) aiConversationPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIConversationGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "conversation not found")
	}

	me := c.MustGet("user").(*models.User)
	if obj.UserId != me.Id {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}

	var body struct {
		Title string `json:"title"`
	}
	ginx.BindJSON(c, &body)

	ginx.NewRender(c).Message(obj.Update(rt.Ctx, body.Title))
}

func (rt *Router) aiConversationDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIConversationGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "conversation not found")
	}

	me := c.MustGet("user").(*models.User)
	if obj.UserId != me.Id {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}

	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

func (rt *Router) aiConversationMessageAdd(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AIConversationGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "conversation not found")
	}

	me := c.MustGet("user").(*models.User)
	if obj.UserId != me.Id {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}

	var msgs []models.AIConversationMessage
	ginx.BindJSON(c, &msgs)

	for i := range msgs {
		msgs[i].ConversationId = id
		ginx.Dangerous(msgs[i].Create(rt.Ctx))
	}

	// Update conversation timestamp
	obj.UpdateTime(rt.Ctx)

	ginx.NewRender(c).Message(nil)
}
