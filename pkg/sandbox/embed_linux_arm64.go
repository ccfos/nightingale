//go:build sandbox_embed && linux && arm64

package sandbox

// Built with `-tags sandbox_embed` for linux/arm64: the bwrap binary and the
// python-base rootfs (tar.gz) are baked into the n9e binary and extracted at
// startup (see rootfs_extract.go). Produce the assets with
// scripts/build-sandbox-assets.sh before building.
// The builder generates the embed declarations only after the assets are ready,
// so go mod vendor can inspect every build tag in a clean checkout. After
// restoring prebuilt assets, run scripts/generate-sandbox-embed.sh <arch>.

func embeddedBwrap() []byte     { return bwrapBin }
func embeddedBaseTarGz() []byte { return baseTarGz }
func embeddedInit() []byte      { return initBin }
