package main

import (
	"reflect"
	"testing"
)

func TestParseArgs(t *testing.T) {
	fwd, child, err := parseArgs([]string{"--forward", "127.0.0.1:18080=/run/n9e-egress.sock", "--", "python3", "/skill/main.py", "--flag"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !reflect.DeepEqual(fwd, []string{"127.0.0.1:18080=/run/n9e-egress.sock"}) {
		t.Errorf("forwards = %v", fwd)
	}
	if !reflect.DeepEqual(child, []string{"python3", "/skill/main.py", "--flag"}) {
		t.Errorf("child = %v", child)
	}

	// No forwards, just a child.
	fwd, child, err = parseArgs([]string{"--", "bash", "/skill/run.sh"})
	if err != nil || len(fwd) != 0 || !reflect.DeepEqual(child, []string{"bash", "/skill/run.sh"}) {
		t.Errorf("no-forward case: fwd=%v child=%v err=%v", fwd, child, err)
	}

	// Multiple forwards.
	fwd, _, err = parseArgs([]string{"--forward", "a=b", "--forward", "c=d", "--", "true"})
	if err != nil || !reflect.DeepEqual(fwd, []string{"a=b", "c=d"}) {
		t.Errorf("multi-forward: fwd=%v err=%v", fwd, err)
	}

	// Unknown argument before "--" is rejected (the engine controls argv).
	if _, _, err := parseArgs([]string{"--bogus", "--", "true"}); err == nil {
		t.Error("unknown flag should error")
	}
}
