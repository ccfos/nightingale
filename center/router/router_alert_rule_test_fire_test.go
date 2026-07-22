package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestFire 准备基于内存 sqlite 的 Router 并预置一个业务组。
// 每次 setup 重置限流表，避免用例间互相影响。
func setupTestFire(t *testing.T) (*Router, *ctx.Context, int64) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.BusiGroup{}, &models.AlertMute{}, &models.AlertSubscribe{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	c := &ctx.Context{DB: db, IsCenter: true}
	rt := &Router{Ctx: c}

	bg := &models.BusiGroup{Name: "infra"}
	if err := db.Create(bg).Error; err != nil {
		t.Fatalf("seed busi group: %v", err)
	}

	testFireMu.Lock()
	testFireLastRun = map[string]time.Time{}
	testFireMu.Unlock()
	return rt, c, bg.Id
}

type testFireStageResp struct {
	Stage  string                 `json:"stage"`
	Status string                 `json:"status"`
	Data   map[string]interface{} `json:"data"`
}

type testFireResp struct {
	Dat struct {
		Event  map[string]interface{} `json:"event"`
		Stages []testFireStageResp    `json:"stages"`
	} `json:"dat"`
	Err string `json:"err"`
}

// callTestFire 直接调用 handler。ginx.Bomb 以 panic 形式中断（生产环境由中间件恢复），
// 这里捕获后以 panicked=true 返回。
func callTestFire(t *testing.T, rt *Router, bgid int64, form AlertRuleTestFireForm) (resp *testFireResp, panicked bool) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body, err := json.Marshal(form)
	if err != nil {
		t.Fatalf("marshal form: %v", err)
	}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", bgid)}}
	c.Set("user", &models.User{Id: 1, Username: "tester"})
	c.Set("username", "tester")

	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	rt.alertRuleTestFire(c)

	resp = &testFireResp{}
	if err := json.Unmarshal(w.Body.Bytes(), resp); err != nil {
		t.Fatalf("unmarshal resp: %v, body: %s", err, w.Body.String())
	}
	return resp, false
}

func stageByName(t *testing.T, resp *testFireResp, name string) testFireStageResp {
	t.Helper()
	for _, s := range resp.Dat.Stages {
		if s.Stage == name {
			return s
		}
	}
	t.Fatalf("stage %q not found in %v", name, resp.Dat.Stages)
	return testFireStageResp{}
}

// 基础链路：mock 样本 + 干跑，七段报告齐全，事件字段合成正确
func TestAlertRuleTestFire_Basic(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:          "cpu high",
			NotifyVersion: 1,
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}
	if resp.Err != "" {
		t.Fatalf("unexpected err: %s", resp.Err)
	}

	wantStages := []string{"synthesize", "effective", "pipeline", "mute", "notify", "subscribe", "side_effects"}
	if len(resp.Dat.Stages) != len(wantStages) {
		t.Fatalf("stages count: got %d, want %d", len(resp.Dat.Stages), len(wantStages))
	}
	for i, name := range wantStages {
		if resp.Dat.Stages[i].Stage != name {
			t.Fatalf("stage[%d]: got %q, want %q", i, resp.Dat.Stages[i].Stage, name)
		}
	}

	if got := resp.Dat.Event["rule_name"]; got != "[TEST] cpu high" {
		t.Fatalf("rule_name: got %v, want [TEST] cpu high", got)
	}
	if got := resp.Dat.Event["severity"]; got != float64(2) {
		t.Fatalf("default severity: got %v, want 2", got)
	}
	if got := resp.Dat.Event["is_recovered"]; got != false {
		t.Fatalf("is_recovered: got %v, want false", got)
	}
	if got := resp.Dat.Event["group_name"]; got != "infra" {
		t.Fatalf("group_name: got %v, want infra", got)
	}

	tags, _ := resp.Dat.Event["tags"].([]interface{})
	joined := fmt.Sprintf("%v", tags)
	if !strings.Contains(joined, "rulename=cpu high") {
		t.Fatalf("tags missing rulename: %v", tags)
	}

	if stageByName(t, resp, "synthesize").Data["sample_source"] != "mock" {
		t.Fatalf("sample_source: want mock")
	}
	if stageByName(t, resp, "effective").Status != "pass" {
		t.Fatalf("effective status: want pass, got %v", stageByName(t, resp, "effective"))
	}
	// NotifyVersion=1 且未配置通知规则：这是新人最常见的坑，应显式警示
	notify := stageByName(t, resp, "notify")
	if notify.Status != "warn" || notify.Data["no_targets"] != true {
		t.Fatalf("notify stage should warn no_targets, got %+v", notify)
	}
}

// 真实样本：labels 进事件标签、内部 label(__name__) 被剔除、trigger_value 取样本值
func TestAlertRuleTestFire_RealSample(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Severity: 1,
		Sample: &TestFireSample{
			Labels: map[string]string{"ident": "web-01", "__name__": "cpu_usage_active"},
			Value:  92.3,
			Query:  "cpu_usage_active > 80",
		},
		Config: models.AlertRule{Name: "cpu high", NotifyVersion: 1},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	if got := stageByName(t, resp, "synthesize").Data["sample_source"]; got != "real" {
		t.Fatalf("sample_source: got %v, want real", got)
	}
	if got := resp.Dat.Event["trigger_value"]; got != "92.3" {
		t.Fatalf("trigger_value: got %v, want 92.3", got)
	}
	if got := resp.Dat.Event["severity"]; got != float64(1) {
		t.Fatalf("severity: got %v, want 1", got)
	}
	tags := fmt.Sprintf("%v", resp.Dat.Event["tags"])
	if !strings.Contains(tags, "ident=web-01") {
		t.Fatalf("tags missing ident: %v", tags)
	}
	if strings.Contains(tags, "__name__") {
		t.Fatalf("tags should not contain internal labels: %v", tags)
	}
}

// annotations 中的 {{$value}} 模板变量应被渲染
func TestAlertRuleTestFire_AnnotationsRendered(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config: models.AlertRule{
			Name:            "cpu high",
			NotifyVersion:   1,
			AnnotationsJSON: map[string]string{"summary": "value is {{$value}}"},
		},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	annotations, _ := resp.Dat.Event["annotations"].(map[string]interface{})
	if got := annotations["summary"]; got != "value is 81.5" {
		t.Fatalf("annotations.summary: got %v, want 'value is 81.5'", got)
	}
	if stageByName(t, resp, "synthesize").Status != "pass" {
		t.Fatalf("synthesize should pass")
	}
}

// 规则被禁用：effective 段警示但链路继续走完
func TestAlertRuleTestFire_DisabledWarn(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend: true,
		Config:   models.AlertRule{Name: "cpu high", Disabled: 1, NotifyVersion: 1},
	})
	if panicked {
		t.Fatal("handler panicked")
	}

	effective := stageByName(t, resp, "effective")
	if effective.Status != "warn" || effective.Data["disabled"] != true {
		t.Fatalf("effective should warn disabled, got %+v", effective)
	}
	if len(resp.Dat.Stages) != 7 {
		t.Fatalf("chain should continue after disabled warn, stages: %d", len(resp.Dat.Stages))
	}
}

// 恢复类型事件：未开启「恢复时通知」时，与真实链路一致跳过发送并警示
func TestAlertRuleTestFire_RecoverEvent(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend:  true,
		EventType: "recover",
		Config:    models.AlertRule{Name: "cpu high", NotifyVersion: 1},
	})
	if panicked {
		t.Fatal("handler panicked")
	}
	if got := resp.Dat.Event["is_recovered"]; got != true {
		t.Fatalf("is_recovered: got %v, want true", got)
	}
	notify := stageByName(t, resp, "notify")
	if notify.Status != "warn" || notify.Data["recover_notify_disabled"] != true {
		t.Fatalf("notify stage should warn recover_notify_disabled, got %+v", notify)
	}
}

// 开启「恢复时通知」的恢复事件正常走通知段
func TestAlertRuleTestFire_RecoverEventNotifyEnabled(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	resp, panicked := callTestFire(t, rt, bgid, AlertRuleTestFireForm{
		SkipSend:  true,
		EventType: "recover",
		Config:    models.AlertRule{Name: "cpu high", NotifyVersion: 1, NotifyRecovered: 1},
	})
	if panicked {
		t.Fatal("handler panicked")
	}
	notify := stageByName(t, resp, "notify")
	if notify.Data["recover_notify_disabled"] == true {
		t.Fatalf("notify stage should not report recover_notify_disabled when enabled, got %+v", notify)
	}
}

// 脱敏：错误串里出现凭据字段名标记时整体隐藏，避免流水线节点凭据经 node_results 泄露
func TestRedactSensitive(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// callback 处理器把整个 config 结构体格式化进 error 的真实形态
		{"failed to send request: dial timeout processor: &{HTTPConfig:{Url:http://x AuthPassword:s3cret Headers:map[Authorization:Bearer t]}}", "(details hidden for security)"},
		{"header contains token abc", "(details hidden for security)"},
		{"connection refused", "connection refused"},
		{"", ""},
	}
	for _, c := range cases {
		if got := redactSensitive(c.in); got != c.want {
			t.Fatalf("redactSensitive(%q): got %q, want %q", c.in, got, c.want)
		}
	}

	// 过长的普通错误被截断
	long := strings.Repeat("a", 500)
	got := redactSensitive(long)
	if len(got) != 303 || !strings.HasSuffix(got, "...") {
		t.Fatalf("long error should be truncated to 303 chars ending with ..., got len %d", len(got))
	}
}

// 同一 用户+业务组 10 秒内重复触发应被限流
func TestAlertRuleTestFire_RateLimit(t *testing.T) {
	rt, _, bgid := setupTestFire(t)

	form := AlertRuleTestFireForm{
		SkipSend: true,
		Config:   models.AlertRule{Name: "cpu high", NotifyVersion: 1},
	}
	if _, panicked := callTestFire(t, rt, bgid, form); panicked {
		t.Fatal("first call should succeed")
	}
	if _, panicked := callTestFire(t, rt, bgid, form); !panicked {
		t.Fatal("second call within 10s should be rate limited")
	}
}
