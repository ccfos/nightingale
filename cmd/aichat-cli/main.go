// Command aichat-cli is a terminal test client for the n9e AI Assistant.
//
// It talks to the v1 service routes (/v1/n9e/assistant/...) which use
// the X-Service-Username header for auth, so it does not depend on the
// frontend or any login session.
//
// Example:
//
//	go run ./cmd/aichat-cli --server http://127.0.0.1:17000 --user root
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// ====================== flags ======================

var (
	flagServer    = flag.String("server", "http://127.0.0.1:17000", "n9e center base url")
	flagUser      = flag.String("user", "root", "X-Service-Username (n9e username to act as)")
	flagBasicAuth = flag.String("basic-auth", "user001:ccc26da7b9aba533cbb263a36c07dcc5", "HTTP Basic auth as user:pass; defaults match etc/config.toml's APIForService.BasicAuth, pass empty to disable")
	flagPage      = flag.String("page", "explorer", "initial page (explorer|dashboards|alert_history|active_alert)")
	flagAction    = flag.String("action", "", "optional action key for the first message (general_chat|alert_query|resource_query|query_generator)")
	flagDsType    = flag.String("ds-type", "", "optional action.param.datasource_type")
	flagDsID      = flag.Int64("ds-id", 0, "optional action.param.datasource_id")
	flagDB        = flag.String("db", "", "optional action.param.database_name")
	flagTable     = flag.String("table", "", "optional action.param.table_name")
	flagChatID    = flag.String("chat-id", "", "resume an existing chat instead of creating a new one")
	flagDebug     = flag.Bool("debug", false, "print every HTTP request/response")
	flagNoColor   = flag.Bool("no-color", false, "disable ANSI colors")
)

// ====================== api types ======================

type apiResp struct {
	Dat json.RawMessage `json:"dat"`
	Err string          `json:"err"`
}

type chatInfo struct {
	ChatID     string `json:"chat_id"`
	Title      string `json:"title"`
	LastUpdate int64  `json:"last_update"`
}

type messageResponse struct {
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
	StreamID    string `json:"stream_id,omitempty"`
	IsFinish    bool   `json:"is_finish"`
	IsFromAI    bool   `json:"is_from_ai"`
}

type messageDetail struct {
	ChatID        string            `json:"chat_id"`
	SeqID         int64             `json:"seq_id"`
	Response      []messageResponse `json:"response"`
	CurStep       string            `json:"cur_step"`
	IsFinish      bool              `json:"is_finish"`
	ErrCode       int               `json:"err_code"`
	ErrMsg        string            `json:"err_msg"`
	ExecutedTools bool              `json:"executed_tools"`
}

type streamMsg struct {
	V string `json:"v"`
	P string `json:"p"`
}

// ====================== http client ======================

type client struct {
	baseURL string
	user    string
	basic   string
	debug   bool
	http    *http.Client
}

func newClient() *client {
	return &client{
		baseURL: strings.TrimRight(*flagServer, "/"),
		user:    *flagUser,
		basic:   *flagBasicAuth,
		debug:   *flagDebug,
		http:    &http.Client{Timeout: 0}, // streams need no timeout; per-call ctx is used instead
	}
}

func (c *client) applyAuth(req *http.Request) {
	if c.basic != "" {
		i := strings.IndexByte(c.basic, ':')
		if i > 0 {
			req.SetBasicAuth(c.basic[:i], c.basic[i+1:])
		}
	}
	if c.user != "" {
		req.Header.Set("X-Service-Username", c.user)
	}
}

// httpError is returned when the server replies with a non-200 status. It
// carries the status code so callers can branch on it (e.g. retry on 409).
type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("http %d: %s", e.status, e.body)
}

// doJSON sends a JSON request and decodes the {"dat":..,"err":..} envelope.
// If out is non-nil, dat is unmarshalled into it.
func (c *client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	var rawBody []byte
	if body != nil {
		var err error
		rawBody, err = json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(rawBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuth(req)

	if c.debug {
		fmt.Fprintf(os.Stderr, "%s %s %s\n", dim(">>"), method, path)
		if len(rawBody) > 0 {
			fmt.Fprintf(os.Stderr, "%s %s\n", dim(">>"), truncate(string(rawBody), 500))
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if c.debug {
		fmt.Fprintf(os.Stderr, "%s %d %s\n", dim("<<"), resp.StatusCode, truncate(string(respBytes), 500))
	}

	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.StatusCode, body: truncate(string(respBytes), 300)}
	}

	var env apiResp
	if err := json.Unmarshal(respBytes, &env); err != nil {
		return fmt.Errorf("decode envelope: %v (body=%s)", err, truncate(string(respBytes), 200))
	}
	if env.Err != "" {
		return errors.New(env.Err)
	}
	if out != nil && len(env.Dat) > 0 {
		if err := json.Unmarshal(env.Dat, out); err != nil {
			return fmt.Errorf("decode dat: %v", err)
		}
	}
	return nil
}

// newChat creates a chat. param can be nil.
func (c *client) newChat(ctx context.Context, page string, param json.RawMessage) (*chatInfo, error) {
	body := map[string]any{"page": page}
	if len(param) > 0 {
		body["param"] = param
	}
	var out chatInfo
	if err := c.doJSON(ctx, "POST", "/v1/n9e/assistant/chat/new", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// sendMessage posts a new message and returns the assigned seq_id.
//
// The server holds a per-chat Redis lock for the entire duration of message
// processing. The lock is released in a deferred call that runs AFTER the
// stream cache's "finish" event has already been sent to the client. So a
// client that fires the next message immediately after seeing finish can
// race the server's lock release and get a 409. We retry briefly on 409
// to absorb that race; any other error is returned immediately.
func (c *client) sendMessage(ctx context.Context, chatID, content, actionKey string, ap actionParam) (int64, error) {
	body := map[string]any{
		"chat_id": chatID,
		"query": map[string]any{
			"content": content,
			"action": map[string]any{
				"key":   actionKey,
				"param": ap,
			},
		},
	}
	var out struct {
		ChatID string `json:"chat_id"`
		SeqID  int64  `json:"seq_id"`
	}

	const maxAttempts = 15 // ~3s of total wait at 200ms backoff
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := c.doJSON(ctx, "POST", "/v1/n9e/assistant/message/new", body, &out)
		if err == nil {
			return out.SeqID, nil
		}
		var herr *httpError
		if !errors.As(err, &herr) || herr.status != http.StatusConflict {
			return 0, err
		}
		// 409 chat busy — wait briefly and retry.
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return 0, errors.New("chat stayed busy after retries")
}

type actionParam struct {
	DatasourceType string `json:"datasource_type,omitempty"`
	DatasourceID   int64  `json:"datasource_id,omitempty"`
	DatabaseName   string `json:"database_name,omitempty"`
	TableName      string `json:"table_name,omitempty"`
}

func (c *client) messageDetail(ctx context.Context, chatID string, seqID int64) (*messageDetail, error) {
	body := map[string]any{"chat_id": chatID, "seq_id": seqID}
	var out messageDetail
	if err := c.doJSON(ctx, "POST", "/v1/n9e/assistant/message/detail", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) messageHistory(ctx context.Context, chatID string) ([]messageDetail, error) {
	body := map[string]any{"chat_id": chatID}
	var out []messageDetail
	if err := c.doJSON(ctx, "POST", "/v1/n9e/assistant/message/history", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) cancelMessage(ctx context.Context, chatID string, seqID int64) error {
	body := map[string]any{"chat_id": chatID, "seq_id": seqID}
	return c.doJSON(ctx, "POST", "/v1/n9e/assistant/message/cancel", body, nil)
}

// streamMessage opens an SSE connection and invokes onMsg for each event.
// It returns when the server closes the stream or ctx is cancelled.
//
// stats counts events seen so callers can distinguish "stream ended cleanly with N events"
// from "stream broke immediately". When debug is enabled, every raw line is dumped to stderr.
func (c *client) streamMessage(ctx context.Context, streamID string, onMsg func(streamMsg)) (events int, err error) {
	body, _ := json.Marshal(map[string]string{"stream_id": streamID})
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/n9e/assistant/stream", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	c.applyAuth(req)

	if c.debug {
		fmt.Fprintf(os.Stderr, "%s POST /v1/n9e/assistant/stream stream_id=%s\n", dim(">>"), streamID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("stream http %d: %s", resp.StatusCode, truncate(string(b), 300))
	}

	gotFinish := false
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if c.debug {
			fmt.Fprintf(os.Stderr, "%s %s\n", dim("sse<"), truncate(line, 300))
		}
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			if strings.TrimSpace(strings.TrimPrefix(line, "event:")) == "finish" {
				gotFinish = true
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var m streamMsg
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			if c.debug {
				fmt.Fprintf(os.Stderr, "%s decode err: %v\n", dim("sse<"), err)
			}
			continue
		}
		events++
		onMsg(m)
	}
	if scErr := scanner.Err(); scErr != nil && !errors.Is(scErr, context.Canceled) {
		return events, scErr
	}
	if !gotFinish {
		// Reached EOF without an explicit "event: finish" — connection was likely
		// torn down mid-stream. Caller should fall back to polling message detail.
		return events, errors.New("connection closed without finish event")
	}
	return events, nil
}

// ====================== display helpers ======================

func ansi(code, s string) string {
	if *flagNoColor {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}
func dim(s string) string  { return ansi("2", s) }
func bold(s string) string { return ansi("1", s) }
func red(s string) string  { return ansi("31", s) }
func cyan(s string) string { return ansi("36", s) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

func shortChatID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ====================== repl ======================

type repl struct {
	c        *client
	chat     *chatInfo
	firstMsg bool
	stdin    *bufio.Reader
	sigCh    chan os.Signal
}

func (r *repl) buildInitialParam() json.RawMessage {
	// Encode optional datasource info into the page param. The schema is page-specific;
	// the explorer page in particular reads datasource_type and datasource_id.
	if *flagDsType == "" && *flagDsID == 0 && *flagDB == "" && *flagTable == "" {
		return nil
	}
	m := map[string]any{}
	if *flagDsType != "" {
		m["datasource_type"] = *flagDsType
	}
	if *flagDsID != 0 {
		m["datasource_id"] = *flagDsID
	}
	if *flagDB != "" {
		m["database_name"] = *flagDB
	}
	if *flagTable != "" {
		m["table_name"] = *flagTable
	}
	b, _ := json.Marshal(m)
	return b
}

func (r *repl) ensureChat(ctx context.Context) error {
	if r.chat != nil {
		return nil
	}
	if *flagChatID != "" {
		r.chat = &chatInfo{ChatID: *flagChatID, Title: "(resumed)"}
		r.firstMsg = false
		return nil
	}
	ch, err := r.c.newChat(ctx, *flagPage, r.buildInitialParam())
	if err != nil {
		return err
	}
	r.chat = ch
	r.firstMsg = true
	fmt.Printf("%s chat_id=%s page=%s\n", dim("[new chat]"), ch.ChatID, *flagPage)
	return nil
}

// resolveStreamID polls message/detail until the stream_id is populated.
// The server creates the message asynchronously; in practice the stream_id is
// available immediately, but we retry briefly to be safe.
func (r *repl) resolveStreamID(ctx context.Context, seqID int64) (string, *messageDetail, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		md, err := r.c.messageDetail(ctx, r.chat.ChatID, seqID)
		if err == nil {
			for _, resp := range md.Response {
				if resp.StreamID != "" {
					return resp.StreamID, md, nil
				}
			}
		}
		if time.Now().After(deadline) {
			if err == nil {
				err = errors.New("stream_id not assigned within 2s")
			}
			return "", nil, err
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (r *repl) actionForFirstMessage() (string, actionParam) {
	if !r.firstMsg {
		return "", actionParam{}
	}
	ap := actionParam{
		DatasourceType: *flagDsType,
		DatasourceID:   *flagDsID,
		DatabaseName:   *flagDB,
		TableName:      *flagTable,
	}
	return *flagAction, ap
}

func (r *repl) sendOnce(ctx context.Context, content string) {
	if err := r.ensureChat(ctx); err != nil {
		fmt.Println(red("[error] " + err.Error()))
		return
	}

	actionKey, ap := r.actionForFirstMessage()
	seqID, err := r.c.sendMessage(ctx, r.chat.ChatID, content, actionKey, ap)
	if err != nil {
		fmt.Println(red("[error] sendMessage: " + err.Error()))
		return
	}
	r.firstMsg = false
	fmt.Println(dim(fmt.Sprintf("[seq=%d] queued", seqID)))

	streamID, _, err := r.resolveStreamID(ctx, seqID)
	if err != nil {
		fmt.Println(red("[error] resolveStream: " + err.Error()))
		return
	}

	// stream
	currentPhase := ""
	switchPhase := func(p string) {
		if p == currentPhase {
			return
		}
		if currentPhase != "" {
			fmt.Println()
		}
		switch p {
		case "reason":
			fmt.Print(dim("[reason] "))
		case "content":
			fmt.Print(bold("[content] "))
		}
		currentPhase = p
	}
	onMsg := func(m streamMsg) {
		switchPhase(m.P)
		if m.P == "reason" {
			fmt.Print(dim(m.V))
		} else {
			fmt.Print(m.V)
		}
	}

	events, streamErr := r.streamWithReconnect(ctx, streamID, onMsg)
	if currentPhase != "" {
		fmt.Println()
	}
	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		fmt.Println(red(fmt.Sprintf("[stream] ended after %d events: %s", events, streamErr.Error())))
		fmt.Println(dim("[stream] falling back to message detail polling ..."))
	}

	// If the user cancelled, tell the server to stop too.
	if errors.Is(ctx.Err(), context.Canceled) {
		_ = r.c.cancelMessage(context.Background(), r.chat.ChatID, seqID)
		fmt.Println(dim("[cancelled]"))
	}

	// Poll detail until the server marks the message finished. EOF on the SSE
	// connection does NOT mean the server is done — the message may still be
	// running in a goroutine. Truth lives in message/detail.
	md, err := r.waitForFinish(seqID, 60*time.Second)
	if err != nil {
		fmt.Println(red("[error] final detail: " + err.Error()))
		return
	}
	r.printFinal(md)
}

// streamWithReconnect wraps streamMessage with automatic reconnection.
//
// The server's http.Server.WriteTimeout (default 40s) closes any SSE
// connection that lives longer than that, regardless of activity. To avoid
// truncated streams for long-running messages, we transparently reconnect
// using the same stream_id whenever the connection ends without an explicit
// "event: finish".
//
// On reconnect, the server's stream cache replays the entire message history
// from index 0 (snapshotted under the cache lock, so any messages produced
// during the gap are also included). We track how many events we've already
// surfaced to the caller and skip exactly that many on each reconnect, so
// onMsg sees each event at most once.
//
// Returned events count is the total distinct events delivered to onMsg.
func (r *repl) streamWithReconnect(ctx context.Context, streamID string, onMsg func(streamMsg)) (int, error) {
	const maxAttempts = 20
	delivered := 0 // count of unique events surfaced to onMsg so far
	for attempt := 0; attempt < maxAttempts; attempt++ {
		skip := delivered
		seenThisAttempt := 0
		wrap := func(m streamMsg) {
			seenThisAttempt++
			if seenThisAttempt <= skip {
				return // already delivered to caller on a previous attempt
			}
			delivered++
			onMsg(m)
		}

		_, err := r.c.streamMessage(ctx, streamID, wrap)
		if err == nil {
			// Clean finish (got "event: finish").
			return delivered, nil
		}
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return delivered, err
		}

		if r.c.debug {
			fmt.Fprintf(os.Stderr, "%s reconnecting (attempt %d/%d, delivered=%d, reason=%v)\n",
				dim("[stream]"), attempt+1, maxAttempts, delivered, err)
		}

		// Brief pause before reconnect; also gives the server a tick to push
		// any new chunks into the cache so the next attempt is productive.
		select {
		case <-ctx.Done():
			return delivered, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return delivered, fmt.Errorf("max stream reconnect attempts (%d) exceeded", maxAttempts)
}

// waitForFinish polls message/detail every 500ms until is_finish=true,
// err_code != 0, or timeout. It always uses a fresh background context so
// it survives a cancelled per-message ctx.
func (r *repl) waitForFinish(seqID int64, timeout time.Duration) (*messageDetail, error) {
	deadline := time.Now().Add(timeout)
	var last *messageDetail
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		md, err := r.c.messageDetail(ctx, r.chat.ChatID, seqID)
		cancel()
		if err == nil {
			last = md
			if md.IsFinish || md.ErrCode != 0 {
				return md, nil
			}
		}
		if time.Now().After(deadline) {
			if last != nil {
				return last, nil // return whatever we have, even if not finished
			}
			if err == nil {
				err = errors.New("timed out waiting for is_finish")
			}
			return nil, err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (r *repl) printFinal(md *messageDetail) {
	fmt.Println(dim("---"))
	if md.ErrCode != 0 {
		fmt.Println(red(fmt.Sprintf("[err_code=%d] %s", md.ErrCode, md.ErrMsg)))
		return
	}
	fmt.Printf("%s seq=%d is_finish=%v executed_tools=%v\n",
		dim("[final]"), md.SeqID, md.IsFinish, md.ExecutedTools)
	for i, resp := range md.Response {
		// reasoning was already streamed, skip its (potentially long) replay
		if resp.ContentType == "reasoning" {
			fmt.Printf("  %s reasoning (%d chars, streamed above)\n", cyan(fmt.Sprintf("#%d", i)), len(resp.Content))
			continue
		}
		fmt.Printf("  %s %s\n", cyan(fmt.Sprintf("#%d", i)), dim(resp.ContentType))
		fmt.Println(indent(resp.Content, "    "))
	}
}

func indent(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// ====================== built-in commands ======================

func (r *repl) handleSlash(ctx context.Context, line string) (handled bool, quit bool) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false, false
	}
	cmd := parts[0]
	switch cmd {
	case "/quit", "/exit", "/q":
		return true, true
	case "/new":
		r.chat = nil
		if err := r.ensureChat(ctx); err != nil {
			fmt.Println(red("[error] " + err.Error()))
		}
		return true, false
	case "/info":
		if r.chat == nil {
			fmt.Println(dim("(no chat)"))
		} else {
			fmt.Printf("chat_id=%s title=%q\n", r.chat.ChatID, r.chat.Title)
		}
		return true, false
	case "/history":
		if r.chat == nil {
			fmt.Println(dim("(no chat)"))
			return true, false
		}
		msgs, err := r.c.messageHistory(ctx, r.chat.ChatID)
		if err != nil {
			fmt.Println(red("[error] " + err.Error()))
			return true, false
		}
		for _, m := range msgs {
			fmt.Printf("%s seq=%d is_finish=%v err_code=%d\n",
				dim("--"), m.SeqID, m.IsFinish, m.ErrCode)
			for _, resp := range m.Response {
				if resp.ContentType == "reasoning" {
					fmt.Printf("  [reasoning] (%d chars)\n", len(resp.Content))
					continue
				}
				fmt.Printf("  [%s]\n%s\n", resp.ContentType, indent(truncate(resp.Content, 800), "    "))
			}
		}
		return true, false
	case "/help", "/?":
		printREPLHelp()
		return true, false
	}
	if strings.HasPrefix(cmd, "/") {
		fmt.Println(red("unknown command: " + cmd + " (try /help)"))
		return true, false
	}
	return false, false
}

func printREPLHelp() {
	fmt.Println(`commands:
  /new       start a new chat (discards current chat handle)
  /info      show current chat info
  /history   list all messages in current chat
  /help      this help
  /quit      exit (Ctrl-D and Ctrl-C at the prompt also exit)
non-command lines are sent as user messages.
during streaming, Ctrl-C cancels the in-flight message;
press Ctrl-C again at the prompt to exit.`)
}

// ====================== main loop ======================

// readLineResult is the outcome of one stdin read in the background reader
// goroutine. Sent over a channel so the main loop can select between input
// and signals.
type readLineResult struct {
	line string
	err  error
}

func (r *repl) run() {
	r.stdin = bufio.NewReader(os.Stdin)
	r.sigCh = make(chan os.Signal, 1)
	signal.Notify(r.sigCh, syscall.SIGINT, syscall.SIGTERM)

	bgCtx := context.Background()
	if err := r.ensureChat(bgCtx); err != nil {
		fmt.Println(red("[error] " + err.Error()))
		return
	}

	// Persistent stdin reader goroutine. We can't cancel a blocking ReadString
	// from the outside, so we run it forever in the background and pipe each
	// completed line through readCh. The main loop selects between this and
	// r.sigCh, which lets Ctrl-C at the prompt exit cleanly.
	readCh := make(chan readLineResult)
	go func() {
		for {
			line, err := r.stdin.ReadString('\n')
			readCh <- readLineResult{line: line, err: err}
			if err != nil {
				return
			}
		}
	}()

	for {
		// Drain any signal that arrived while we were processing the previous
		// message, so a stale Ctrl-C doesn't bounce us out of the prompt
		// before the user can react.
		select {
		case <-r.sigCh:
		default:
		}

		fmt.Printf("%s ", bold(fmt.Sprintf("[%s]>", shortChatID(r.chat.ChatID))))

		var line string
		select {
		case res := <-readCh:
			if res.err != nil {
				if errors.Is(res.err, io.EOF) {
					fmt.Println()
					return
				}
				fmt.Println(red("[error] read: " + res.err.Error()))
				return
			}
			line = strings.TrimSpace(res.line)
		case <-r.sigCh:
			// Ctrl-C at the idle prompt → exit cleanly.
			fmt.Println()
			return
		}

		if line == "" {
			continue
		}

		if handled, quit := r.handleSlash(bgCtx, line); handled {
			if quit {
				return
			}
			continue
		}

		// Per-message cancelable context wired to SIGINT. While a message is
		// in flight, Ctrl-C cancels it (which makes sendOnce return); the
		// next iteration's prompt-level Ctrl-C handler then takes over.
		msgCtx, msgCancel := context.WithCancel(bgCtx)
		sigDone := make(chan struct{})
		go func() {
			select {
			case <-r.sigCh:
				msgCancel()
			case <-sigDone:
			}
		}()
		r.sendOnce(msgCtx, line)
		close(sigDone)
		msgCancel()
	}
}

// ====================== entrypoint ======================

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "aichat-cli — terminal test client for n9e AI Assistant")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Type /help inside the REPL for built-in commands.")
	}
	flag.Parse()

	r := &repl{c: newClient()}
	r.run()
}
