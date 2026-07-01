package sandbox

// engineFactory builds an Engine for the resolved config + probed capabilities.
// It returns an error when the backend cannot run on this host (e.g. the bwrap
// binary is missing) so the control plane can degrade.
type engineFactory func(cfg Config, caps Capabilities) (Engine, error)

// engineRegistry holds the backends compiled into this build. Each engine file
// registers itself from init(); the unsafe engine is present on every OS, the
// Linux engines only under //go:build linux. Selection (New) looks engines up
// here by name, so adding a backend is just a registerEngine call.
var engineRegistry = map[string]engineFactory{}

func registerEngine(name string, f engineFactory) {
	engineRegistry[name] = f
}

func lookupEngine(name string) (engineFactory, bool) {
	f, ok := engineRegistry[name]
	return f, ok
}
