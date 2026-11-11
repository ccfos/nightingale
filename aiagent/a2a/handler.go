package a2a

import (
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/eventqueue"
	a2astore "github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// HandlerOptions tunes the A2A REST handler. The zero value is valid.
type HandlerOptions struct {
	// TaskStore persists tasks across the cluster so tasks/get and
	// tasks/resubscribe survive process restarts and LB-induced instance
	// switches. When nil, the SDK's in-memory default is used (per-process,
	// non-durable).
	TaskStore a2astore.Store
}

// NewHTTPHandler builds the REST/HTTP+JSON A2A handler that should be mounted
// at the path advertised in AgentCard.SupportedInterfaces. Mount it under any
// gin group that has the n9e tokenAuth middleware applied; the executor reads
// the user from request.Context (see WithUser).
func NewHTTPHandler(backend AssistantBackend, opts HandlerOptions) http.Handler {
	options := []a2asrv.RequestHandlerOption{}
	if opts.TaskStore != nil {
		options = append(options, a2asrv.WithTaskStore(opts.TaskStore))
	}
	// eventqueue 默认缓冲仅 32：长输出轮次（多指标查询 + 长正文）事件量大，executor 的
	// Write 要等所有订阅者收到广播才返回，慢消费者会把 executor 拖住数分钟、stream
	// 消费停滞，直接决定 message:send 同步响应延迟（agent 已完成落库但响应迟迟不返
	// 回，客户端超时）。加大缓冲缓解瞬时积压；持续背压的根治见 bridge 事件合并。
	queueMgr := eventqueue.NewInMemoryManager(eventqueue.WithQueueBufferSize(1024))
	options = append(options, a2asrv.WithEventQueueManager(queueMgr))
	requestHandler := a2asrv.NewHandler(NewExecutor(backend), options...)
	return a2asrv.NewRESTHandler(requestHandler)
}
