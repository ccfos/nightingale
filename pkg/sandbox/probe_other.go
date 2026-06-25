//go:build !linux

package sandbox

// probeCapabilities on non-Linux returns a kernel-isolation-incapable inventory.
// Every kernel-primitive sandbox is Linux-only (§2.2/§6); the main n9e binary
// still runs everywhere, only Skill execution is gated. selectTier maps this to
// TierDisabled, and New() degrades to the unsafe engine only when dev_mode=true.
func probeCapabilities() Capabilities {
	c := baseCaps()
	c.note("non-Linux host (%s): kernel-isolation engines unavailable", c.OS)
	return c
}
