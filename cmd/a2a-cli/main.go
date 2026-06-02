// Command a2a-cli is a terminal test client for the n9e A2A endpoint.
//
// It uses the official a2a-go SDK to:
//  1. Resolve the AgentCard at /.well-known/agent-card.json
//  2. Send a streaming message and print every event the agent emits,
//     decoding the two metadata conventions the n9e server tags parts with:
//     reasoning ("adk_thought") and structured payloads ("n9e_content_type").
//  3. (Optionally) call tasks/get to verify the TaskStore can recover the
//     task after the stream finishes — useful for sanity-checking the
//     multi-instance / resubscribe story.
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
//	# Resolve a full AgentCard URL directly (no well-known path appended)
//	go run ./cmd/a2a-cli \
//	    --server http://host:9000/api/fc-model/a2a/agent-card.json \
//	    --card-path "" --token <token> --message hi
//
//	# Verify TaskStore: stream + then re-fetch via tasks/get
//	go run ./cmd/a2a-cli --server http://127.0.0.1:17000 --token <t> --message hi --get
package main

import (
	"context"
	"encoding/base64"
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

// Part metadata keys the n9e A2A server tags streamed parts with. Kept in sync
// with aiagent/a2a (server side) and fc-model-server's tools_n9e (peer client).
const (
	metaThought     = "adk_thought"      // bool: reasoning / chain-of-thought, rendered separately
	metaContentType = "n9e_content_type" // string: part.Text() carries a JSON structured payload
)

func main() {
	var (
		server    = flag.String("server", "http://127.0.0.1:17000", "n9e base URL (or full AgentCard URL together with --card-path \"\")")
		token     = flag.String("token", "", "X-User-Token issued in the n9e UI")
		header    = flag.String("token-header", "X-User-Token", "auth header carrying the user token")
		cardPath  = flag.String("card-path", "/.well-known/agent-card.json", "AgentCard path joined to --server; pass \"\" to resolve --server as-is")
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
	p := &streamPrinter{}
	for event, err := range client.SendStreamingMessage(ctx, req) {
		if err != nil {
			log.Fatalf("stream error: %v", err)
		}
		p.handle(event)
	}
	p.printAnswer()
	return p.taskID
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

// streamPrinter renders streamed events in a human-readable form and
// accumulates the clean (non-thought, non-structured) text into a final answer.
type streamPrinter struct {
	seq    int
	taskID a2a.TaskID
	answer strings.Builder
}

func (p *streamPrinter) handle(event a2a.Event) {
	p.seq++
	switch e := event.(type) {
	case *a2a.Task:
		p.taskID = e.ID
		fmt.Printf("[%03d] task      id=%s context=%s state=%s%s\n",
			p.seq, e.ID, e.ContextID, e.Status.State, metaSuffix(e.Metadata))
	case *a2a.TaskStatusUpdateEvent:
		ts := ""
		if e.Status.Timestamp != nil {
			ts = " @" + e.Status.Timestamp.Format(time.RFC3339)
		}
		msg := ""
		if e.Status.Message != nil {
			msg = " msg=" + oneLine(concatText(e.Status.Message.Parts))
		}
		fmt.Printf("[%03d] status    state=%s%s%s%s\n",
			p.seq, e.Status.State, ts, msg, metaSuffix(e.Metadata))
	case *a2a.TaskArtifactUpdateEvent:
		p.handleArtifact(e)
	case *a2a.Message:
		fmt.Printf("[%03d] message   %s\n", p.seq, oneLine(concatText(e.Parts)))
	default:
		fmt.Printf("[%03d] unknown   type=%T %s\n", p.seq, event, mustJSON(event))
	}
}

func (p *streamPrinter) handleArtifact(e *a2a.TaskArtifactUpdateEvent) {
	if e.Artifact == nil {
		fmt.Printf("[%03d] artifact  <nil>\n", p.seq)
		return
	}
	flags := ""
	if e.Append {
		flags += " append"
	}
	if e.LastChunk {
		flags += " last"
	}
	for _, part := range e.Artifact.Parts {
		if part == nil {
			continue
		}
		kind, content := describePart(part)
		fmt.Printf("[%03d] artifact  id=%s%s [%s] %s\n",
			p.seq, e.Artifact.ID, flags, kind, oneLine(content))
		if isPlainAnswer(part) {
			p.answer.WriteString(part.Text())
		}
	}
}

func (p *streamPrinter) printAnswer() {
	fmt.Println("\n== final answer ==")
	if ans := strings.TrimSpace(p.answer.String()); ans != "" {
		fmt.Println(ans)
	} else {
		fmt.Println("(empty)")
	}
}

// describePart classifies a part by its n9e metadata first, then by the
// a2a content union, returning a short type label and a rendered value.
func describePart(part *a2a.Part) (kind, content string) {
	if isThought(part.Metadata) {
		return "thought", part.Text()
	}
	if ct := contentType(part.Metadata); ct != "" {
		return "n9e_content_type=" + ct, structuredContent(part)
	}
	return renderContent(part)
}

func renderContent(part *a2a.Part) (kind, content string) {
	switch v := part.Content.(type) {
	case a2a.Text:
		return "text", string(v)
	case a2a.Data:
		return "data", compactJSON(v.Value)
	case a2a.Raw:
		return "raw", fmt.Sprintf("%d bytes base64=%s", len(v), base64.StdEncoding.EncodeToString([]byte(v)))
	case a2a.URL:
		return "url", string(v)
	default:
		return fmt.Sprintf("%T", part.Content), ""
	}
}

// structuredContent decodes an n9e_content_type part, whose Text() holds a JSON
// payload, and re-marshals it compactly. Falls back to the raw text if it is
// not valid JSON.
func structuredContent(part *a2a.Part) string {
	raw := part.Text()
	var data any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return raw
	}
	out := compactJSON(data)
	if part.MediaType != "" {
		out = "media_type=" + part.MediaType + " " + out
	}
	return out
}

// isPlainAnswer reports whether a part is ordinary answer text (not reasoning
// and not a structured payload), i.e. the content worth assembling into the
// final response.
func isPlainAnswer(part *a2a.Part) bool {
	if isThought(part.Metadata) || contentType(part.Metadata) != "" {
		return false
	}
	_, ok := part.Content.(a2a.Text)
	return ok
}

func isThought(meta map[string]any) bool {
	v, ok := meta[metaThought].(bool)
	return ok && v
}

func contentType(meta map[string]any) string {
	if v, ok := meta[metaContentType].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func metaSuffix(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	return " meta=" + compactJSON(meta)
}

func concatText(parts a2a.ContentParts) string {
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

func compactJSON(v any) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<json err: %v>", err)
	}
	return string(b)
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("<json err: %v>", err)
	}
	return string(b)
}
