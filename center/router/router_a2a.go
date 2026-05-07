package router

import (
	"context"
	"errors"
	"net/http"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/a2a"
	a2ataskstore "github.com/ccfos/nightingale/v6/aiagent/a2a/taskstore"
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
	// Translate the router-side typed sentinel into the A2A SDK's canonical
	// "task not found" error at the package boundary, so aiagent/a2a never has
	// to know about router internals. Other errors propagate untouched.
	if err := b.rt.CancelAssistantMessageInternal(ctx, chatID, seqID); err != nil {
		if errors.Is(err, ErrMessageNotInflight) {
			return a2asdk.ErrTaskNotFound
		}
		return err
	}
	return nil
}

func (b *a2aBackend) CheckChatOwner(chatID string, userID int64) error {
	_, err := models.AssistantChatCheckOwner(b.rt.Ctx, chatID, userID)
	return err
}

func (b *a2aBackend) StreamBus() aiagent.StreamBus {
	return b.rt.streamBus
}

func (b *a2aBackend) MessageSnapshot(ctx context.Context, chatID string, seqID int64) (*models.AssistantMessage, error) {
	return models.MsgStateGet(ctx, b.rt.Redis, chatID, seqID)
}

// configRegisterA2A mounts the AgentCard, A2A and MCP endpoints. The HTTP path
// "/.well-known/agent.json" is reserved for AgentCard discovery; A2A is mounted
// at /a2a, MCP at /mcp. Both endpoints reuse the n9e tokenAuth middleware.
func (rt *Router) configRegisterA2A(r *gin.Engine) {
	if rt.HTTP.A2A.Disable {
		return
	}
	if !rt.HTTP.TokenAuth.Enable {
		logger.Warning("[A2A] HTTP.TokenAuth.Enable=false — AgentCard advertises X-User-Token apiKey but the server will only accept JWT credentials. Enable HTTP.TokenAuth so the advertised auth scheme actually works.")
	}

	backend := &a2aBackend{rt: rt}

	tokenHeader := rt.HTTP.TokenAuth.HeaderUserTokenKey
	if tokenHeader == "" {
		tokenHeader = DefaultTokenKey
	}

	// Build a Redis-backed TaskStore so tasks/get and tasks/resubscribe
	// survive process restarts and LB-induced instance switches. All n9e
	// center instances share the same Redis, so multi-instance correctness
	// comes for free via the store's Lua-CAS update path.
	taskStore := a2ataskstore.NewRedisStore(rt.Redis, a2ataskstore.Options{
		User: func(ctx context.Context) (string, error) {
			user := a2a.UserFromContext(ctx)
			if user == nil {
				return "", nil
			}
			return user.Username, nil
		},
	})

	handlerOpts := a2a.HandlerOptions{TaskStore: taskStore}

	// AgentCard is public — it carries no instance-specific secrets, only a
	// description of the agent's capabilities. Spec requires it to be
	// reachable without authentication so clients can discover.
	cardHandler := gin.WrapH(a2a.AgentCardHandler(a2a.AgentCardOptions{
		BaseURL:         rt.HTTP.A2A.BaseURL,
		A2APath:         "/a2a",
		TokenHeaderName: tokenHeader,
	}))
	// Canonical A2A v0.3+ path; alias kept for older clients.
	r.GET("/.well-known/agent-card.json", cardHandler)
	r.GET("/.well-known/agent.json", cardHandler)

	// The SDK's internal http.ServeMux is registered at root paths like
	// /message:send /message:stream /tasks/{id}, so we MUST strip the /a2a
	// mount prefix before delegating; otherwise the SDK's mux sees
	// /a2a/message:send and 404s.
	a2aHandler := http.StripPrefix("/a2a", a2a.NewHTTPHandler(backend, handlerOpts))
	a2aGroup := r.Group("/a2a")
	a2aGroup.Use(rt.tokenAuth(), rt.user(), rt.injectA2AUser(), rt.streamingDeadline())
	a2aGroup.Any("", gin.WrapH(a2aHandler))
	a2aGroup.Any("/*proxyPath", gin.WrapH(a2aHandler))

	if !rt.HTTP.A2A.DisableMCP {
		mcpHandler := http.StripPrefix("/mcp", a2a.NewMCPHandler(backend))
		mcpGroup := r.Group("/mcp")
		mcpGroup.Use(rt.tokenAuth(), rt.user(), rt.injectA2AUser(), rt.streamingDeadline())
		mcpGroup.Any("", gin.WrapH(mcpHandler))
		mcpGroup.Any("/*proxyPath", gin.WrapH(mcpHandler))
	}
}

// injectA2AUser pulls *models.User from gin.Context (set by rt.user()) and
// stuffs it into request.Context so the a2a executor / mcp handler can read
// it without depending on gin. The streaming write-deadline relaxation is
// applied separately via streamingDeadline() so each middleware does one
// thing.
func (rt *Router) injectA2AUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get("user")
		if !ok {
			c.Abort()
			ginx.Bomb(http.StatusUnauthorized, "unauthorized")
			return
		}
		// A bad type assertion here means rt.user() ran but stuffed something
		// other than *models.User into the context — that's a middleware-chain
		// wiring bug, not a credential problem. Surface it as 500 so it can't
		// be confused with normal auth failures during incident triage.
		user, ok := v.(*models.User)
		if !ok || user == nil {
			c.Abort()
			ginx.Bomb(http.StatusInternalServerError, "a2a: user middleware returned wrong type")
			return
		}
		c.Request = c.Request.WithContext(a2a.WithUser(c.Request.Context(), user))
		c.Next()
	}
}

// streamingDeadline relaxes the per-connection write deadline for endpoints
// that may stream longer than http.Server.WriteTimeout (40s default). A2A SSE
// streams (message:stream, tasks/{id}:subscribe) and long MCP responses can
// be silent for minutes during a single ReAct turn; without this the TCP
// connection is closed mid-stream and the SDK's REST encoder fails with
// "write tcp: i/o timeout".
func (rt *Router) streamingDeadline() gin.HandlerFunc {
	return func(c *gin.Context) {
		clearWriteDeadline(c.Writer)
		c.Next()
	}
}
