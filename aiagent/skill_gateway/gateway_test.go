package skillgateway

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// newTestGateway builds a Gateway WITHOUT Start() (which needs a DB to resolve
// the user). The bound principal is an admin, so user.CheckPerm short-circuits
// without a DB and the authorize/protocol/limiter logic is exercised in isolation.
func newTestGateway(cfg Config) *Gateway {
	if cfg.RatePerSec == 0 {
		cfg.RatePerSec = 1000
	}
	if cfg.RateBurst == 0 {
		cfg.RateBurst = 100
	}
	return &Gateway{
		execID:  "se_test",
		skill:   "demo",
		user:    &models.User{Id: 1, Username: "admin", RolesLst: []string{"Admin"}},
		isAdmin: true,
		cfg:     cfg,
		limiter: newTokenBucket(cfg.RatePerSec, cfg.RateBurst),
	}
}

func TestAuthorizeGates(t *testing.T) {
	// Envelope: list_alert_rules (alert:read) allowed only when granted.
	g := newTestGateway(Config{GrantableAPI: []string{"alert:read"}})
	if err := g.authorize(ops["list_alert_rules"]); err != nil {
		t.Errorf("granted alert:read should authorize: %v", err)
	}
	if err := g.authorize(ops["list_datasources"]); err == nil {
		t.Error("datasource:read not in envelope should be denied")
	}

	// Hard deny wins even inside a broad envelope.
	g = newTestGateway(Config{GrantableAPI: []string{"*:read"}, DenyAPI: []string{"alert:read"}})
	if err := g.authorize(ops["list_alert_rules"]); err == nil {
		t.Error("hard-denied alert:read must be rejected despite *:read envelope")
	}
	if err := g.authorize(ops["list_targets"]); err != nil {
		t.Errorf("target:read under *:read envelope should authorize: %v", err)
	}

	// whoami is identity-only: always allowed, even with an empty envelope.
	g = newTestGateway(Config{})
	if err := g.authorize(ops["whoami"]); err != nil {
		t.Errorf("whoami must always authorize: %v", err)
	}
}

// roundTrip sends one request line to handleConn over an in-memory pipe and
// returns the decoded response.
func roundTrip(t *testing.T, g *Gateway, reqJSON string) response {
	t.Helper()
	cli, srv := net.Pipe()
	go g.handleConn(srv)
	defer cli.Close()
	_ = cli.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(cli, reqJSON+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	line, err := bufio.NewReader(cli).ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var resp response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return resp
}

func TestHandleWhoami(t *testing.T) {
	g := newTestGateway(Config{})
	resp := roundTrip(t, g, `{"op":"whoami"}`)
	if !resp.OK {
		t.Fatalf("whoami failed: %s", resp.Error)
	}
	data, _ := resp.Data.(map[string]any)
	if data["username"] != "admin" || data["is_admin"] != true {
		t.Fatalf("whoami data = %+v, want admin identity", data)
	}
}

func TestHandleUnknownOp(t *testing.T) {
	g := newTestGateway(Config{})
	resp := roundTrip(t, g, `{"op":"rm_minus_rf"}`)
	if resp.OK || resp.Error == "" {
		t.Fatalf("unknown op should fail, got %+v", resp)
	}
}

func TestHandleDeniedOp(t *testing.T) {
	// alert:read NOT in the (empty) envelope → list_alert_rules denied before it
	// ever reaches the DB.
	g := newTestGateway(Config{})
	resp := roundTrip(t, g, `{"op":"list_alert_rules"}`)
	if resp.OK || resp.Error == "" {
		t.Fatalf("ungranted op should be denied, got %+v", resp)
	}
}

func TestRateLimit(t *testing.T) {
	g := newTestGateway(Config{RatePerSec: 0.001, RateBurst: 1})
	if resp := roundTrip(t, g, `{"op":"whoami"}`); !resp.OK {
		t.Fatalf("first call should pass: %s", resp.Error)
	}
	if resp := roundTrip(t, g, `{"op":"whoami"}`); resp.OK {
		t.Fatal("second call should be rate-limited")
	}
}
