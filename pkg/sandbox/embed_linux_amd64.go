//go:build sandbox_embed && linux && amd64

package sandbox

import _ "embed"

// Built with `-tags sandbox_embed` for linux/amd64. See embed_linux_arm64.go.

//go:embed embedassets/linux_amd64/bwrap
var bwrapBin []byte

//go:embed embedassets/linux_amd64/python-base.tar.gz
var baseTarGz []byte

//go:embed embedassets/linux_amd64/n9e-sandbox-init
var initBin []byte

func embeddedBwrap() []byte     { return bwrapBin }
func embeddedBaseTarGz() []byte { return baseTarGz }
func embeddedInit() []byte      { return initBin }
