package pconf

import (
	"strings"
	"testing"

	"github.com/koding/multiconfig"
)

func TestTargetTagAppendDefaults(t *testing.T) {
	var config struct {
		Pushgw Pushgw
	}

	loader := multiconfig.MultiLoader(
		&multiconfig.TagLoader{},
		&multiconfig.TOMLLoader{Reader: strings.NewReader("[Pushgw]\n")},
	)
	if err := loader.Load(&config); err != nil {
		t.Fatalf("load config defaults: %v", err)
	}

	if !config.Pushgw.EnableTargetTagAppend {
		t.Fatal("EnableTargetTagAppend should default to true")
	}
	if !config.Pushgw.EnableTargetHostTagAppend {
		t.Fatal("EnableTargetHostTagAppend should default to true")
	}
}

func TestTargetTagAppendExplicitlyDisabled(t *testing.T) {
	var config struct {
		Pushgw Pushgw
	}

	loader := multiconfig.MultiLoader(
		&multiconfig.TagLoader{},
		&multiconfig.TOMLLoader{Reader: strings.NewReader(`[Pushgw]
EnableTargetTagAppend = false
EnableTargetHostTagAppend = false
`)},
	)
	if err := loader.Load(&config); err != nil {
		t.Fatalf("load config overrides: %v", err)
	}

	if config.Pushgw.EnableTargetTagAppend {
		t.Fatal("EnableTargetTagAppend should honor explicit false")
	}
	if config.Pushgw.EnableTargetHostTagAppend {
		t.Fatal("EnableTargetHostTagAppend should honor explicit false")
	}
}
