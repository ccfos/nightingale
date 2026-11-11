package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
)

func TestTestRoundsWithRecovery(t *testing.T) {
	ev := &models.AlertCurEvent{Hash: "h", IsRecovered: true}
	rounds := testRounds([]*models.AlertCurEvent{ev}, true)
	if len(rounds) != 2 || rounds[0][0].IsRecovered || !rounds[1][0].IsRecovered {
		t.Fatalf("with_recovery must send firing first and then recovered: %+v", rounds)
	}
	if rounds[0][0].Hash != "h" || rounds[1][0].Hash != "h" {
		t.Fatal("both rounds must keep the same event hash")
	}
	if !ev.IsRecovered {
		t.Fatal("the original event must not be mutated")
	}
	if got := testRounds([]*models.AlertCurEvent{ev}, false); len(got) != 1 || got[0][0] != ev {
		t.Fatal("without recovery the events are sent once as-is")
	}
}

func TestWithTestNonceKeepsParams(t *testing.T) {
	in := map[string]interface{}{"project_key": "OPS"}
	out := withTestNonce(in)
	if out["project_key"] != "OPS" || out[provider.TestNonceParam] == "" || out[provider.TestNonceParam] == nil {
		t.Fatalf("nonce param missing: %+v", out)
	}
	if _, ok := in[provider.TestNonceParam]; ok {
		t.Fatal("input params must not be mutated")
	}
	if withTestNonce(nil)[provider.TestNonceParam] == nil {
		t.Fatal("nil params should still get a nonce")
	}
}

// 模拟事件的 Hash 必须保持固定：PagerDuty 的 dedup_key 就是它，测试恢复要能对上测试触发
func TestMockEventHashStaysFixed(t *testing.T) {
	a := buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 2})
	b := buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 2, MockIsRecovered: true})
	if a.Hash != "notify-channel-test-mock-event" || a.Hash != b.Hash {
		t.Fatalf("mock event hash must stay fixed, got %q %q", a.Hash, b.Hash)
	}
}

func TestBuildTestTplContentPlainForJira(t *testing.T) {
	nc := &models.NotifyChannelConfig{Ident: "jira", RequestType: models.RequestTypeJira}
	events := []*models.AlertCurEvent{{RuleName: `a "b"`}}
	got, err := buildTestTplContent(nc, map[string]string{"content": "{{$event.RuleName}}\nx"}, events, "http://n9e")
	if err != nil {
		t.Fatal(err)
	}
	if got["content"] != "a \"b\"\nx" {
		t.Fatalf("jira test content must be rendered as plain text, got %q", got["content"])
	}
}
