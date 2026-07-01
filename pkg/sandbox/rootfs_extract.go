package sandbox

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractEmbeddedAssets materializes the embedded bwrap binary + python-base
// rootfs (when the binary was built with -tags sandbox_embed, §9.3) under
// dataDir, and returns their on-disk paths. It returns ("","",nil) when nothing
// is embedded — the common default-build case — so callers can fall back to an
// external bwrap (PATH) + Rootfs.Path. Extraction is content-addressed and
// idempotent: re-running with the same assets is a no-op.
func extractEmbeddedAssets(dataDir string) (bwrapPath, basePath, initPath string, err error) {
	bw := embeddedBwrap()
	base := embeddedBaseTarGz()
	ini := embeddedInit()
	if len(bw) == 0 && len(base) == 0 && len(ini) == 0 {
		return "", "", "", nil
	}
	if len(bw) > 0 {
		if bwrapPath, err = writeEmbeddedExecutable(dataDir, "bwrap", bw); err != nil {
			return "", "", "", fmt.Errorf("extract embedded bwrap: %w", err)
		}
	}
	if len(base) > 0 {
		if basePath, err = extractEmbeddedBase(dataDir, base); err != nil {
			return "", "", "", fmt.Errorf("extract embedded python-base: %w", err)
		}
	}
	if len(ini) > 0 {
		// The egress forwarder (§10.2); extracted next to bwrap and bind-mounted
		// into the sandbox by the bubblewrap engine when network=proxy.
		if initPath, err = writeEmbeddedExecutable(dataDir, "n9e-sandbox-init", ini); err != nil {
			return "", "", "", fmt.Errorf("extract embedded n9e-sandbox-init: %w", err)
		}
	}
	return bwrapPath, basePath, initPath, nil
}

func shortHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:12]
}

// writeEmbeddedExecutable writes an embedded helper binary to
// dataDir/bin/<name>-<hash> (exec bit set), content-addressed so it is written
// once and reused, and returns its path. Used for bwrap and the egress
// forwarder (n9e-sandbox-init).
func writeEmbeddedExecutable(dataDir, name string, bin []byte) (string, error) {
	dir := filepath.Join(dataDir, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+"-"+shortHash(bin))
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(bin)) {
		return path, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// extractEmbeddedBase unpacks the python-base tar.gz into a content-addressed,
// immutable directory rootfs/python-base@<hash>, marked complete with a .ok
// file so a half-extracted tree is never reused. It also creates the standard
// sandbox mount-point dirs the bwrap engine binds onto (the base contract — see
// engine_bwrap_linux.go), so any base works regardless of whether it shipped
// them.
func extractEmbeddedBase(dataDir string, targz []byte) (string, error) {
	dir := filepath.Join(dataDir, "rootfs", "python-base@"+shortHash(targz))
	// Marker lives BESIDE the base dir (not inside it) so it never appears as
	// /.ok inside the sandbox root.
	okMarker := dir + ".ok"
	if _, err := os.Stat(okMarker); err == nil {
		return dir, nil // already extracted
	}
	_ = os.RemoveAll(dir) // clear any partial leftover
	_ = os.Remove(okMarker)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	if err := untarGz(dir, targz); err != nil {
		return "", err
	}
	// Base contract: bwrap binds these read-only/read-write onto the rootfs; the
	// rootfs is ro so the mountpoints must pre-exist. /run hosts the control-plane
	// tmpfs (egress + gateway sockets, the forwarder binary, §10.2/§12.1).
	for _, mp := range []string{"skill", "input", "workspace", "output", "run"} {
		if err := os.MkdirAll(filepath.Join(dir, mp), 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(okMarker, []byte(shortHash(targz)), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func untarGz(dest string, targz []byte) error {
	gz, err := gzip.NewReader(strings.NewReader(string(targz)))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		rel := filepath.Clean("/" + hdr.Name) // anchor then strip leading /
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || rel == "." {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			return fmt.Errorf("tar entry escapes root: %q", hdr.Name)
		}
		target := filepath.Join(dest, rel)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)&0o777|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			linkTarget := filepath.Join(dest, filepath.Clean("/"+hdr.Linkname))
			_ = os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				// Fall back to a copy if hardlink fails (cross-dir/edge cases).
				if data, rerr := os.ReadFile(linkTarget); rerr == nil {
					_ = os.WriteFile(target, data, os.FileMode(hdr.Mode)&0o777)
				}
			}
		default:
			// Skip char/block devices, fifos, etc. — bwrap mounts a fresh /dev.
		}
	}
}
