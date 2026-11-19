//go:build sandbox_embed && linux && amd64

package sandbox

// Built with `-tags sandbox_embed` for linux/amd64. See embed_linux_arm64.go.

func embeddedBwrap() []byte     { return bwrapBin }
func embeddedBaseTarGz() []byte { return baseTarGz }
func embeddedInit() []byte      { return initBin }
