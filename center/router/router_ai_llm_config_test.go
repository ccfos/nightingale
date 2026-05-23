package router

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/llmconfig"
)

func TestTranslateProbeErrorAuth(t *testing.T) {
	msg := translateProbeError("en", &llmconfig.ProbeError{
		Kind:       llmconfig.ProbeErrorAuth,
		StatusCode: 401,
		Detail:     "invalid key",
	})

	if !strings.Contains(msg, "authentication failed (HTTP 401)") {
		t.Fatalf("unexpected translated message: %q", msg)
	}
	if !strings.Contains(msg, "invalid key") {
		t.Fatalf("unexpected translated message: %q", msg)
	}
}

func TestTranslateProbeErrorModel(t *testing.T) {
	msg := translateProbeError("en", &llmconfig.ProbeError{
		Kind:   llmconfig.ProbeErrorModel,
		Model:  "bad-model",
		Detail: "model not found",
	})

	if !strings.Contains(msg, "model(bad-model): model not found") {
		t.Fatalf("unexpected translated message: %q", msg)
	}
}
