package sandbox

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSafeJoin(t *testing.T) {
	root := "/var/lib/n9e/sandbox/skill"
	bad := []string{
		"../escape",
		"a/../../escape",
		"/etc/passwd",
		"..",
		"a/b/../../../c",
		"with\x00nul",
		"",
	}
	for _, p := range bad {
		if _, err := SafeJoin(root, p); err == nil {
			t.Errorf("SafeJoin(%q) should have been rejected", p)
		}
	}
	good := map[string]string{
		"main.py":      root + "/main.py",
		"sub/dir/x.py": root + "/sub/dir/x.py",
		"./a.sh":       root + "/a.sh",
		"a/./b":        root + "/a/b",
	}
	for in, want := range good {
		got, err := SafeJoin(root, in)
		if err != nil {
			t.Errorf("SafeJoin(%q) unexpected err: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("SafeJoin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCappedBufferTruncates(t *testing.T) {
	c := &cappedBuffer{max: 4}
	n, _ := c.Write([]byte("aaa"))
	if n != 3 || c.truncated {
		t.Fatalf("first write: n=%d truncated=%v", n, c.truncated)
	}
	// second write overflows the 4-byte cap
	n, _ = c.Write([]byte("bbbb"))
	if n != 4 {
		t.Fatalf("second write should report all bytes consumed, got %d", n)
	}
	if !c.truncated {
		t.Fatal("expected truncated=true after overflow")
	}
	if got := c.buf.String(); got != "aaab" {
		t.Fatalf("buffer = %q, want capped %q", got, "aaab")
	}
}

func TestConfigPreCheckDefaults(t *testing.T) {
	var c Config
	c.PreCheck()
	if c.Engine != "auto" {
		t.Errorf("Engine default = %q, want auto", c.Engine)
	}
	if c.DefaultPolicy.TimeoutSeconds != defaultTimeoutSeconds {
		t.Errorf("default timeout = %d", c.DefaultPolicy.TimeoutSeconds)
	}
	if c.Admission.MaxConcurrent != defaultMaxConcurrent {
		t.Errorf("default max concurrent = %d", c.Admission.MaxConcurrent)
	}
	// idempotent
	prev := c
	c.PreCheck()
	if c.Engine != prev.Engine || c.DefaultPolicy.MemoryMB != prev.DefaultPolicy.MemoryMB {
		t.Error("PreCheck not idempotent")
	}
}

func TestSelectTier(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		caps Capabilities
		want Tier
	}{
		{"non-linux", Config{}, Capabilities{OS: "darwin"}, TierDisabled},
		{"full-linux", Config{}, Capabilities{OS: "linux", UserNS: true, Seccomp: true, CgroupV2Delegated: true}, TierBubblewrap},
		{"confined-needs-flag", Config{}, Capabilities{OS: "linux", Seccomp: true, Landlock: true}, TierDisabled},
		{"confined-with-flag", Config{ContainerAsBoundary: true}, Capabilities{OS: "linux", Seccomp: true, Landlock: true}, TierConfined},
		{"insufficient", Config{}, Capabilities{OS: "linux"}, TierDisabled},
	}
	for _, tc := range cases {
		got, reason := selectTier(tc.cfg, tc.caps)
		if got != tc.want {
			t.Errorf("%s: selectTier = %v (%s), want %v", tc.name, got, reason, tc.want)
		}
	}
}

func TestConfigEngineToName(t *testing.T) {
	for in, want := range map[string]string{
		"unsafe":             EngineUnsafe,
		"bubblewrap":         EngineBwrap,
		"bwrap":              EngineBwrap,
		"confined":           EngineConfined,
		"container-confined": EngineConfined,
	} {
		got, ok := configEngineToName(in)
		if !ok || got != want {
			t.Errorf("configEngineToName(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := configEngineToName("nonsense"); ok {
		t.Error("unknown engine should not resolve")
	}
}

func TestAdmissionMemoryReject(t *testing.T) {
	a := newAdmission(AdmissionConfig{MaxConcurrent: 4, MaxTotalMemoryMB: 100})
	rel, err := a.acquire(context.Background(), 80)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	// 80 + 40 > 100 → reject
	if _, err := a.acquire(context.Background(), 40); err == nil {
		t.Fatal("expected memory-budget rejection")
	}
	rel() // free the first reservation
	// now 40 fits
	rel2, err := a.acquire(context.Background(), 40)
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	rel2()
}

func TestAdmissionConcurrencyQueueTimeout(t *testing.T) {
	a := newAdmission(AdmissionConfig{MaxConcurrent: 1, MaxTotalMemoryMB: 10000})
	rel, err := a.acquire(context.Background(), 10)
	if err != nil {
		t.Fatalf("first slot acquire failed: %v", err)
	}
	defer rel()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := a.acquire(ctx, 10); err == nil {
		t.Fatal("expected queue timeout when no slot free")
	}
}

func TestUnsafeEngineRun(t *testing.T) {
	e := &unsafeEngine{}
	res, err := e.Run(context.Background(), ExecSpec{
		SkillName: "t",
		Command:   []string{"sh", "-c", "printf hi; printf err 1>&2; exit 3"},
		Resources: ResourceSpec{Timeout: 5 * time.Second, StdoutMax: 1 << 20, StderrMax: 1 << 20},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if string(res.Stdout) != "hi" {
		t.Errorf("stdout = %q, want hi", res.Stdout)
	}
	if string(res.Stderr) != "err" {
		t.Errorf("stderr = %q, want err", res.Stderr)
	}
}

func TestUnsafeEngineTimeout(t *testing.T) {
	e := &unsafeEngine{}
	res, err := e.Run(context.Background(), ExecSpec{
		Command:   []string{"sh", "-c", "sleep 5"},
		Resources: ResourceSpec{Timeout: 150 * time.Millisecond, StdoutMax: 1024, StderrMax: 1024},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !res.Timeout || res.KilledBy != KilledByTimeout {
		t.Errorf("expected timeout kill, got timeout=%v killedBy=%q", res.Timeout, res.KilledBy)
	}
}

func TestUnsafeEngineStdoutCap(t *testing.T) {
	e := &unsafeEngine{}
	res, err := e.Run(context.Background(), ExecSpec{
		Command:   []string{"sh", "-c", "printf 'aaaaaaaaaa'"}, // 10 bytes
		Resources: ResourceSpec{Timeout: 5 * time.Second, StdoutMax: 4, StderrMax: 1024},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if !res.StdoutTruncated || string(res.Stdout) != "aaaa" {
		t.Errorf("expected stdout capped to 4, got %q truncated=%v", res.Stdout, res.StdoutTruncated)
	}
}

func TestUnsafeEngineCleanEnv(t *testing.T) {
	t.Setenv("SECRET_LEAK", "should-not-appear")
	e := &unsafeEngine{}
	res, err := e.Run(context.Background(), ExecSpec{
		Command:   []string{"sh", "-c", "echo $SECRET_LEAK"},
		Env:       map[string]string{"FOO": "bar"},
		Resources: ResourceSpec{Timeout: 5 * time.Second, StdoutMax: 1024, StderrMax: 1024},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if strings.Contains(string(res.Stdout), "should-not-appear") {
		t.Errorf("host env leaked into sandbox: %q", res.Stdout)
	}
}

func TestSandboxExplicitUnsafeFailOpen(t *testing.T) {
	// fail-open default: explicit unsafe runs without dev_mode.
	s := New(Config{Engine: "unsafe", DataDir: t.TempDir()})
	if !s.Enabled() || s.EngineName() != EngineUnsafe {
		t.Fatalf("explicit unsafe should be enabled by default (fail-open): enabled=%v engine=%q reason=%s",
			s.Enabled(), s.EngineName(), s.DisabledReason())
	}
	// RequireIsolation is the safety ceiling: it refuses the unsafe floor even
	// when unsafe is requested explicitly.
	s2 := New(Config{Engine: "unsafe", RequireIsolation: true, DataDir: t.TempDir()})
	if s2.Enabled() {
		t.Fatal("RequireIsolation=true must refuse explicit unsafe")
	}
	if _, err := s2.Run(context.Background(), ExecSpec{Command: []string{"true"}}); !IsDisabled(err) {
		t.Fatalf("expected DisabledError, got %v", err)
	}
}

func TestSandboxAutoFailOpenToUnsafe(t *testing.T) {
	// On a host with no usable isolation engine (darwin, or linux without a
	// bwrap+rootfs), auto degrades to unsafe-exec by default so scripts still run.
	s := New(Config{Engine: "auto", DataDir: t.TempDir()})
	if !s.Enabled() || s.EngineName() != EngineUnsafe {
		t.Fatalf("auto should fail-open to unsafe on a no-isolation host: enabled=%v engine=%q",
			s.Enabled(), s.EngineName())
	}
	if s.Tier() != TierUnsafe {
		t.Errorf("tier=%v, want unsafe", s.Tier())
	}
	// The same host with RequireIsolation refuses to run.
	if New(Config{Engine: "auto", RequireIsolation: true, DataDir: t.TempDir()}).Enabled() {
		t.Fatal("RequireIsolation=true must disable when only unsafe is available")
	}
}

func TestSandboxDisabledWhenConfigDisabled(t *testing.T) {
	s := New(Config{Disabled: true, Engine: "unsafe", DevMode: true, DataDir: t.TempDir()})
	if s.Enabled() {
		t.Fatal("disabled=true must disable the sandbox")
	}
}

func TestSandboxRunUnsafeDevEndToEnd(t *testing.T) {
	s := New(Config{Disabled: false, Engine: "unsafe", DevMode: true, DataDir: t.TempDir()})
	if !s.Enabled() {
		t.Fatalf("dev unsafe should be enabled: %s", s.DisabledReason())
	}
	if s.EngineName() != EngineUnsafe {
		t.Fatalf("engine = %q, want unsafe", s.EngineName())
	}
	res, err := s.Run(context.Background(), ExecSpec{
		SkillName: "demo",
		Command:   []string{"sh", "-c", "echo ok"},
		Resources: ResourceSpec{Timeout: 5 * time.Second, StdoutMax: 1024, StderrMax: 1024},
	})
	if err != nil {
		t.Fatalf("run err: %v", err)
	}
	if strings.TrimSpace(string(res.Stdout)) != "ok" || res.Engine != EngineUnsafe {
		t.Fatalf("unexpected result: stdout=%q engine=%q", res.Stdout, res.Engine)
	}
}

// fakeEngine is a no-op Engine used to drive resolveEngine's ladder/policy
// deterministically, independent of host capabilities and real engine deps.
type fakeEngine struct{ name string }

func (f fakeEngine) Name() string      { return f.name }
func (f fakeEngine) Caps() EngineCaps  { return EngineCaps{} }
func (f fakeEngine) Run(context.Context, ExecSpec) (ExecResult, error) {
	return ExecResult{Engine: f.name}, nil
}

// swapEngineRegistry replaces the global registry with fakes for the four
// ladder engines: a fake succeeds iff feasible[name] is true. Restored on
// cleanup. White-box: only valid inside package sandbox tests.
func swapEngineRegistry(t *testing.T, feasible map[string]bool) {
	t.Helper()
	saved := engineRegistry
	t.Cleanup(func() { engineRegistry = saved })
	engineRegistry = map[string]engineFactory{}
	for _, name := range strengthOrder {
		n := name
		ok := feasible[n]
		engineRegistry[n] = func(cfg Config, caps Capabilities) (Engine, error) {
			if ok {
				return fakeEngine{name: n}, nil
			}
			return nil, fmt.Errorf("fake %s infeasible", n)
		}
	}
}

func TestResolveEngineLadder(t *testing.T) {
	cases := []struct {
		name       string
		engine     string // cfg.Engine
		requireIso bool
		feasible   map[string]bool
		wantEngine string // "" means disabled
		wantTier   Tier
	}{
		{"auto-picks-bwrap", "auto", false, map[string]bool{EngineBwrap: true, EngineUnsafe: true}, EngineBwrap, TierBubblewrap},
		{"auto-no-cgroupv2-still-bwrap", "", false, map[string]bool{EngineBwrap: true, EngineUnsafe: true}, EngineBwrap, TierBubblewrap},
		{"auto-picks-strongest-runsc", "auto", false, map[string]bool{EngineRunsc: true, EngineBwrap: true, EngineUnsafe: true}, EngineRunsc, TierStrong},
		{"auto-confined-when-bwrap-infeasible", "auto", false, map[string]bool{EngineConfined: true, EngineUnsafe: true}, EngineConfined, TierConfined},
		{"auto-failopen-unsafe", "auto", false, map[string]bool{EngineUnsafe: true}, EngineUnsafe, TierUnsafe},
		{"auto-requireiso-disabled", "auto", true, map[string]bool{EngineUnsafe: true}, "", TierDisabled},
		{"explicit-bwrap-degrades-unsafe", "bubblewrap", false, map[string]bool{EngineUnsafe: true}, EngineUnsafe, TierUnsafe},
		{"explicit-bwrap-requireiso-disabled", "bubblewrap", true, map[string]bool{EngineUnsafe: true}, "", TierDisabled},
		{"explicit-unsafe-enabled", "unsafe", false, map[string]bool{EngineUnsafe: true}, EngineUnsafe, TierUnsafe},
		{"explicit-unsafe-requireiso-disabled", "unsafe", true, map[string]bool{EngineUnsafe: true}, "", TierDisabled},
		{"nothing-feasible-disabled", "auto", false, map[string]bool{}, "", TierDisabled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			swapEngineRegistry(t, tc.feasible)
			s := &Sandbox{cfg: Config{Engine: tc.engine, RequireIsolation: tc.requireIso}, caps: Capabilities{OS: "linux"}}
			s.tier, s.tierReason = selectTier(s.cfg, s.caps)
			s.resolveEngine()

			if tc.wantEngine == "" {
				if s.Enabled() {
					t.Fatalf("want disabled, got engine=%q", s.EngineName())
				}
				if s.DisabledReason() == "" {
					t.Error("disabled but DisabledReason is empty")
				}
				if s.Tier() != TierDisabled {
					t.Errorf("tier=%v, want disabled", s.Tier())
				}
				return
			}
			if !s.Enabled() {
				t.Fatalf("want engine=%q, got disabled: %s", tc.wantEngine, s.DisabledReason())
			}
			if s.EngineName() != tc.wantEngine {
				t.Errorf("engine=%q, want %q", s.EngineName(), tc.wantEngine)
			}
			if s.Tier() != tc.wantTier {
				t.Errorf("tier=%v, want %v", s.Tier(), tc.wantTier)
			}
		})
	}
}
