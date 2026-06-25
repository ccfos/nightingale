//go:build !(sandbox_embed && linux && (amd64 || arm64))

package sandbox

// Default builds carry NO embedded sandbox assets — the bwrap binary and
// python-base rootfs come from the host (an installed bwrap + Rootfs.Path).
// Release builds opt into embedding with `-tags sandbox_embed` for a supported
// linux/<arch>, which selects embed_linux_<arch>.go instead of this stub and
// bakes the per-arch assets into the binary (design §9.3). This keeps the
// default repo build free of committed multi-MB binaries.
func embeddedBwrap() []byte     { return nil }
func embeddedBaseTarGz() []byte { return nil }
