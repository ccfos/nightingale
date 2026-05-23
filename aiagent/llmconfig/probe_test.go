package llmconfig

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

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
