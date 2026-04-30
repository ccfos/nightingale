package router

import (
	"context"
	"net/http"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/a2a"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// a2aBackend adapts *Router into the narrow surface expected by aiagent/a2a.
// Keeps the package boundary clean: aiagent/a2a never imports center/router.
type a2aBackend struct {
	rt *Router
}

func (b *a2aBackend) EnsureAssistantChat(userID int64, chatID string, page models.AssistantPageInfo) (*models.AssistantChat, error) {
	return b.rt.EnsureAssistantChat(userID, chatID, page)
}

func (b *a2aBackend) StartAssistantMessage(userID int64, chat *models.AssistantChat, query models.AssistantMessageQuery, lang string) (*a2a.MessageStartResult, int, error) {
	res, status, err := b.rt.StartAssistantMessage(userID, chat, query, lang)
	if err != nil {
		return nil, status, err
	}
	return &a2a.MessageStartResult{
		ChatID:   res.ChatID,
		SeqID:    res.SeqID,
		StreamID: res.StreamID,
	}, 0, nil
}

func (b *a2aBackend) CancelAssistantMessage(ctx context.Context, chatID string, seqID int64) error {
	return b.rt.CancelAssistantMessageInternal(ctx, chatID, seqID)
}

func (b *a2aBackend) LatestAssistantMessageSeqID(chatID string) (int64, error) {
	return models.AssistantMessageMaxSeqID(b.rt.Ctx, chatID)
}

func (b *a2aBackend) StreamBus() aiagent.StreamBus {
	return b.rt.streamBus
}

// configRegisterA2A mounts the AgentCard, A2A and MCP endpoints. The HTTP path
// "/.well-known/agent.json" is reserved for AgentCard discovery; A2A is mounted
// at /a2a, MCP at /mcp. Both endpoints reuse the n9e tokenAuth middleware.
func (rt *Router) configRegisterA2A(r *gin.Engine) {
	if rt.HTTP.A2A.Disable {
		return
	}
	if !rt.HTTP.TokenAuth.Enable {
		logger.Warning("[A2A] HTTP.A2A is enabled but HTTP.TokenAuth is not — A2A/MCP endpoints will reject every request. Set HTTP.A2A.Disable=true or enable HTTP.TokenAuth.")
	}

	backend := &a2aBackend{rt: rt}

	tokenHeader := rt.HTTP.TokenAuth.HeaderUserTokenKey
	if tokenHeader == "" {
		tokenHeader = DefaultTokenKey
	}

	// AgentCard is public — it carries no instance-specific secrets, only a
	// description of the agent's capabilities. Spec requires it to be
	// reachable without authentication so clients can discover.
	r.GET("/.well-known/agent.json", gin.WrapH(a2a.AgentCardHandler(a2a.AgentCardOptions{
		BaseURL:         rt.HTTP.A2A.BaseURL,
		A2APath:         "/a2a",
		TokenHeaderName: tokenHeader,
	})))

	a2aGroup := r.Group("/a2a")
	a2aGroup.Use(rt.tokenAuth(), rt.user(), rt.injectA2AUser())
	a2aGroup.Any("", gin.WrapH(a2a.NewHTTPHandler(backend)))
	a2aGroup.Any("/*proxyPath", gin.WrapH(a2a.NewHTTPHandler(backend)))

	if !rt.HTTP.A2A.DisableMCP {
		mcpGroup := r.Group("/mcp")
		mcpGroup.Use(rt.tokenAuth(), rt.user(), rt.injectA2AUser())
		mcpGroup.Any("", gin.WrapH(a2a.NewMCPHandler(backend)))
		mcpGroup.Any("/*proxyPath", gin.WrapH(a2a.NewMCPHandler(backend)))
	}
}

// injectA2AUser pulls *models.User from gin.Context (set by rt.user()) and
// stuffs it into request.Context so the a2a executor / mcp handler can read
// it without depending on gin.
func (rt *Router) injectA2AUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get("user")
		if !ok {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
			return
		}
		user, ok := v.(*models.User)
		if !ok || user == nil {
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
			return
		}
		c.Request = c.Request.WithContext(a2a.WithUser(c.Request.Context(), user))
		c.Next()
	}
}
