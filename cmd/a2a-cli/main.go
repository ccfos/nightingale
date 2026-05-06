// Command a2a-cli is a terminal test client for the n9e A2A endpoint.
//
// It uses the official a2a-go SDK to:
//   1. Resolve the AgentCard at /.well-known/agent-card.json
//   2. Send a streaming message and print every event the agent emits
//   3. (Optionally) call tasks/get to verify the TaskStore can recover the
//      task after the stream finishes — useful for sanity-checking the
//      multi-instance / resubscribe story.
//
// Examples:
//
//	# Streaming chat against a local n9e
//	go run ./cmd/a2a-cli \
//	    --server http://127.0.0.1:17000 \
//	    --token  <your X-User-Token> \
//	    --message "查看 prod 业务组当前正在告警的事件"
//
//	# Continue an existing conversation
//	go run ./cmd/a2a-cli \
//	    --server http://127.0.0.1:17000 \
//	    --token <token> \
//	    --context-id <chat-id> \
//	    --message "进一步分析其中第一条"
//
//	# Verify TaskStore: stream + then re-fetch via tasks/get
//	go run ./cmd/a2a-cli --server http://127.0.0.1:17000 --token <t> --message hi --get
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

func main() {
	var (
		server    = flag.String("server", "http://127.0.0.1:17000", "n9e base URL")
		token     = flag.String("token", "", "X-User-Token issued in the n9e UI")
		header    = flag.String("token-header", "X-User-Token", "auth header carrying the user token")
		cardPath  = flag.String("card-path", "/.well-known/agent-card.json", "AgentCard path on the server")
		message   = flag.String("message", "你好", "user message text to send")
		contextID = flag.String("context-id", "", "optional: continue an existing chat (== n9e chat_id)")
		modelID   = flag.Int64("model-id", 0, "optional: override the LLM model_id (passed via metadata)")
		streaming = flag.Bool("stream", true, "use SendStreamingMessage; false uses SendMessage")
		doGet     = flag.Bool("get", false, "after streaming, call tasks/get to verify TaskStore")
		timeout   = flag.Duration("timeout", 5*time.Minute, "overall request timeout")
	)
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "missing --token. Get one from n9e UI -> User Profile -> API Tokens")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// 1) Resolve AgentCard. The n9e AgentCard endpoint does not require auth,
	// but passing the token here doesn't hurt and matches what production
	// clients do.
	card, err := agentcard.DefaultResolver.Resolve(ctx, *server,
		agentcard.WithPath(*cardPath),
		agentcard.WithRequestHeader(*header, *token),
	)
	if err != nil {
		log.Fatalf("resolve agent card: %v", err)
	}
	fmt.Printf("== AgentCard ==\n%s\n\n", mustJSON(card))

	// 2) Build a client from the resolved card. The SDK auto-selects the
	// best transport advertised in SupportedInterfaces (REST/JSON-RPC/gRPC).
	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}

	// 3) Attach the auth header so every transport call carries it.
	ctx = a2aclient.AttachServiceParams(ctx, a2aclient.ServiceParams{
		*header: []string{*token},
	})

	// 4) Compose the message. ContextID maps 1:1 to n9e's chat_id.
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(*message))
	if *contextID != "" {
		msg.ContextID = *contextID
	}

	req := &a2a.SendMessageRequest{Message: msg}
	if *modelID != 0 {
		req.Metadata = map[string]any{"model_id": *modelID}
	}

	var lastTaskID a2a.TaskID
	if *streaming {
		lastTaskID = runStream(ctx, client, req)
	} else {
		lastTaskID = runOnce(ctx, client, req)
	}

	// 5) Optional: tasks/get to confirm TaskStore is wired correctly. The
	// task should return with the final state populated.
	if *doGet && lastTaskID != "" {
		fmt.Printf("\n== tasks/get %s ==\n", lastTaskID)
		got, err := client.GetTask(ctx, &a2a.GetTaskRequest{ID: lastTaskID})
		if err != nil {
			log.Fatalf("tasks/get: %v", err)
		}
		fmt.Println(mustJSON(got))
	}
}

func runStream(ctx context.Context, client *a2aclient.Client, req *a2a.SendMessageRequest) a2a.TaskID {
	fmt.Println("== SendStreamingMessage ==")
	var taskID a2a.TaskID
	for event, err := range client.SendStreamingMessage(ctx, req) {
		if err != nil {
			log.Fatalf("stream error: %v", err)
		}
		taskID = handleEvent(event, taskID)
	}
	return taskID
}

func runOnce(ctx context.Context, client *a2aclient.Client, req *a2a.SendMessageRequest) a2a.TaskID {
	fmt.Println("== SendMessage (non-streaming) ==")
	res, err := client.SendMessage(ctx, req)
	if err != nil {
		log.Fatalf("send: %v", err)
	}
	switch r := res.(type) {
	case *a2a.Task:
		fmt.Println(mustJSON(r))
		return r.ID
	case *a2a.Message:
		fmt.Println(mustJSON(r))
	}
	return ""
}

// handleEvent prints one streamed event in a human-readable form.
// adk_thought-tagged parts are rendered separately as <thought> ... </thought>
// to mirror how ADK clients display reasoning.
func handleEvent(event a2a.Event, prev a2a.TaskID) a2a.TaskID {
	switch e := event.(type) {
	case *a2a.Task:
		fmt.Printf("[task    ] id=%s context=%s state=%s\n", e.ID, e.ContextID, e.Status.State)
		return e.ID
	case *a2a.TaskStatusUpdateEvent:
		extra := ""
		if e.Status.Message != nil {
			extra = " msg=" + concatParts(e.Status.Message.Parts)
		}
		fmt.Printf("[status  ] state=%s%s\n", e.Status.State, extra)
	case *a2a.TaskArtifactUpdateEvent:
		text := concatParts(e.Artifact.Parts)
		isThought := false
		for _, p := range e.Artifact.Parts {
			if p == nil {
				continue
			}
			if v, ok := p.Meta()["adk_thought"].(bool); ok && v {
				isThought = true
				break
			}
		}
		tag := "artifact"
		if isThought {
			tag = "thought "
		}
		last := ""
		if e.LastChunk {
			last = " (last)"
		}
		fmt.Printf("[%s] id=%s%s: %s\n", tag, e.Artifact.ID, last, oneLine(text))
	case *a2a.Message:
		fmt.Printf("[message ] %s\n", concatParts(e.Parts))
	}
	return prev
}

func concatParts(parts a2a.ContentParts) string {
	var sb strings.Builder
	for _, p := range parts {
		if p == nil {
			continue
		}
		sb.WriteString(p.Text())
	}
	return sb.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<json err: %v>", err)
	}
	return string(b)
}
