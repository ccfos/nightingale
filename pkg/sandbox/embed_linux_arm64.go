//go:build sandbox_embed && linux && arm64

package sandbox

import _ "embed"

// Built with `-tags sandbox_embed` for linux/arm64: the bwrap binary and the
// python-base rootfs (tar.gz) are baked into the n9e binary and extracted at
// startup (see rootfs_extract.go). Produce the assets with
// scripts/build-sandbox-assets.sh before building.

//go:embed embedassets/linux_arm64/bwrap
var bwrapBin []byte

//go:embed embedassets/linux_arm64/python-base.tar.gz
var baseTarGz []byte

func embeddedBwrap() []byte     { return bwrapBin }
func embeddedBaseTarGz() []byte { return baseTarGz }
