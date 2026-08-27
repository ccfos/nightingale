package llmconfig

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// newProbeServer 起一个固定返回 body 的假上游，供连通性探测测试使用。
func newProbeServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestProbeTruncatedResponseIsSuccess：推理模型（o1 系列关不掉思考、Azure 自定义部署名
// 下的 gpt-5 注入不了 reasoning_effort）会把探测预算全花在 reasoning 上，正文为空、
// finish_reason=length。这轮请求本身是通的，探测不能报「无内容」。
func TestProbeTruncatedResponseIsSuccess(t *testing.T) {
	srv := newProbeServer(t, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"length"}]}`)

	if err := Test(&models.AILLMConfig{APIType: "openai", APIURL: srv.URL, Model: "o1"}); err != nil {
		t.Fatalf("truncated response should count as a successful probe, got %v", err)
	}
}

// TestProbeEmptyContentWithoutTruncationFails：正常收尾却一个字都没有，仍然要报「无内容」。
func TestProbeEmptyContentWithoutTruncationFails(t *testing.T) {
	srv := newProbeServer(t, `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`)

	err := Test(&models.AILLMConfig{APIType: "openai", APIURL: srv.URL, Model: "gpt-4o"})
	probeErr, ok := err.(*ProbeError)
	if !ok {
		t.Fatalf("expected ProbeError, got %T (%v)", err, err)
	}
	if probeErr.Kind != ProbeErrorNoContent {
		t.Fatalf("unexpected kind: %q", probeErr.Kind)
	}
}

func TestParseProviderStatusError(t *testing.T) {
	statusCode, raw, ok := parseProviderStatusError("OpenAI API error (status 404): not found")
	if !ok {
		t.Fatal("expected status error to be parsed")
	}
	if statusCode != 404 {
		t.Fatalf("unexpected status code: %d", statusCode)
	}
	if raw != "not found" {
		t.Fatalf("unexpected raw body: %q", raw)
	}
}

func TestClassifyProbeErrorHTTPStatus(t *testing.T) {
	err := classifyProbeError(&models.AILLMConfig{APIURL: "https://api.openai.com", Model: "gpt-4o"},
		assertErr("OpenAI API error (status 404): endpoint missing"))

	probeErr, ok := err.(*ProbeError)
	if !ok {
		t.Fatalf("expected ProbeError, got %T", err)
	}
	if probeErr.Kind != ProbeErrorEndpointNotFound {
		t.Fatalf("unexpected kind: %q", probeErr.Kind)
	}
	if probeErr.APIURL != "https://api.openai.com" || probeErr.Detail != "endpoint missing" || probeErr.StatusCode != 404 {
		t.Fatalf("unexpected probe error: %#v", probeErr)
	}
}

func TestClassifyProbeErrorProviderMessage(t *testing.T) {
	err := classifyProbeError(&models.AILLMConfig{Model: "bad-model"}, assertErr("Claude API error: model not found"))

	probeErr, ok := err.(*ProbeError)
	if !ok {
		t.Fatalf("expected ProbeError, got %T", err)
	}
	if probeErr.Kind != ProbeErrorModel {
		t.Fatalf("unexpected kind: %q", probeErr.Kind)
	}
	if probeErr.Model != "bad-model" || probeErr.Detail != "model not found" {
		t.Fatalf("unexpected probe error: %#v", probeErr)
	}
}

func TestFormatHTTPErrorAuth(t *testing.T) {
	err := formatHTTPError(401, "https://api.openai.com", "unauthorized")
	if err.Kind != ProbeErrorAuth {
		t.Fatalf("unexpected kind: %q", err.Kind)
	}
	if err.StatusCode != 401 || err.Detail != "unauthorized" {
		t.Fatalf("unexpected probe error: %#v", err)
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
