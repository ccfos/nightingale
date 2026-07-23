package router

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupAlertRulePutFieldsTest 准备一个基于内存 sqlite 的 Router，并预置一条带
// 附加信息(annotations)和回调地址(callbacks)的告警规则。
func setupAlertRulePutFieldsTest(t *testing.T) (*Router, *ctx.Context, int64) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AlertRule{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	c := &ctx.Context{DB: db}
	rt := &Router{Ctx: c}

	rule := &models.AlertRule{
		GroupId:     1,
		Name:        "test-rule",
		Annotations: `{"env":"prod","team":"infra"}`,
		Callbacks:   "http://a.com http://b.com",
		UpdateBy:    "creator",
		UpdateAt:    1000,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("seed alert rule: %v", err)
	}

	return rt, c, rule.Id
}

// callPutFields 直接调用 alertRulePutFields 处理函数，模拟一次批量更新请求。
func callPutFields(t *testing.T, rt *Router, form alertRuleFieldForm) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body, err := json.Marshal(form)
	if err != nil {
		t.Fatalf("marshal form: %v", err)
	}
	c.Request = httptest.NewRequest("PUT", "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("username", "editor")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("alertRulePutFields panicked: %v", r)
		}
	}()
	rt.alertRulePutFields(c)
}

func getAnnotations(t *testing.T, c *ctx.Context, id int64) map[string]string {
	t.Helper()
	ar, err := models.AlertRuleGetById(c, id)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if ar == nil {
		t.Fatalf("rule %d not found", id)
	}
	return ar.AnnotationsJSON
}

// annotations_add(新增)一个新键：原有附加信息必须保留，同时追加新键。
// 修复前，特殊 action 合并后又被通用流程用本次提交内容覆盖，最终只剩 severity 一个键。
func TestAlertRulePutFields_AnnotationsAdd_NewKey(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "annotations_add",
		Fields: map[string]interface{}{
			"annotations": map[string]interface{}{"severity": "high"},
		},
	})

	got := getAnnotations(t, c, id)
	want := map[string]string{"env": "prod", "team": "infra", "severity": "high"}
	if !annotationsEqual(got, want) {
		t.Fatalf("annotations_add(new key): got %v, want %v", got, want)
	}
}

// annotations_add(新增)一个同名键：应更新该键的值，其余键保持不变。
func TestAlertRulePutFields_AnnotationsAdd_SameKey(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "annotations_add",
		Fields: map[string]interface{}{
			"annotations": map[string]interface{}{"env": "staging"},
		},
	})

	got := getAnnotations(t, c, id)
	want := map[string]string{"env": "staging", "team": "infra"}
	if !annotationsEqual(got, want) {
		t.Fatalf("annotations_add(same key): got %v, want %v", got, want)
	}
}

// annotations_del(删除)一个键：仅删除指定键，其余键保留。
// 修复前，删除后又被通用流程覆盖，最终反而只剩被"删除"的那个键。
func TestAlertRulePutFields_AnnotationsDel(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "annotations_del",
		Fields: map[string]interface{}{
			"annotations": map[string]interface{}{"team": ""},
		},
	})

	got := getAnnotations(t, c, id)
	want := map[string]string{"env": "prod"}
	if !annotationsEqual(got, want) {
		t.Fatalf("annotations_del: got %v, want %v", got, want)
	}
}

// callback_add(新增回调)：在原有回调地址基础上追加，不覆盖。
func TestAlertRulePutFields_CallbackAdd(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "callback_add",
		Fields: map[string]interface{}{
			"callbacks": "http://c.com",
		},
	})

	ar, err := models.AlertRuleGetById(c, id)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	want := "http://a.com http://b.com http://c.com"
	if ar.Callbacks != want {
		t.Fatalf("callback_add: got %q, want %q", ar.Callbacks, want)
	}
}

// callback_del(删除回调)：仅删除目标地址，保留其余地址。
func TestAlertRulePutFields_CallbackDel(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "callback_del",
		Fields: map[string]interface{}{
			"callbacks": "http://a.com",
		},
	})

	ar, err := models.AlertRuleGetById(c, id)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	// http://a.com 被删除，仅保留 http://b.com（DB2FE 会按空格重新切分）
	if len(ar.CallbacksJSON) != 1 || ar.CallbacksJSON[0] != "http://b.com" {
		t.Fatalf("callback_del: got %v, want [http://b.com]", ar.CallbacksJSON)
	}
}

// 无特殊 action 的普通批量更新(cover)：通用字段写入路径应正常生效。
func TestAlertRulePutFields_CoverStillWorks(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "",
		Fields: map[string]interface{}{
			"append_tags": "service=n9e mod=api",
		},
	})

	ar, err := models.AlertRuleGetById(c, id)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if ar.AppendTags != "service=n9e mod=api" {
		t.Fatalf("cover append_tags: got %q, want %q", ar.AppendTags, "service=n9e mod=api")
	}
}

// 任何批量更新都必须刷新 update_by/update_at，否则引擎不会拉取到规则变更。
func TestAlertRulePutFields_UpdatesModifierAndTime(t *testing.T) {
	rt, c, id := setupAlertRulePutFieldsTest(t)

	callPutFields(t, rt, alertRuleFieldForm{
		Ids:    []int64{id},
		Action: "annotations_add",
		Fields: map[string]interface{}{
			"annotations": map[string]interface{}{"severity": "high"},
		},
	})

	ar, err := models.AlertRuleGetById(c, id)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	if ar.UpdateBy != "editor" {
		t.Fatalf("update_by not refreshed: got %q, want %q", ar.UpdateBy, "editor")
	}
	if ar.UpdateAt == 1000 {
		t.Fatalf("update_at not refreshed: still %d", ar.UpdateAt)
	}
}

func annotationsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || bv != v {
			return false
		}
	}
	return true
}
