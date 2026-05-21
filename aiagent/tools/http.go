package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
)

func init() {
	register(defs.HTTPFetch, httpFetch)
}

const (
	httpFetchDefaultMaxBytes = 1 << 20 // 1 MiB
	httpFetchHardMaxBytes    = 8 << 20 // 8 MiB
	httpFetchDefaultTimeout  = 10
	httpFetchMaxTimeout      = 60
	httpFetchMaxRedirects    = 5

	// HTTPFetchTempFilePrefix is the basename prefix used by http_fetch when
	// save_to_file=true. Downstream tools (preview_prom_rule_yaml /
	// import_prom_rule_yaml) refuse any payload_file whose basename doesn't
	// start with this prefix — keeps the LLM from passing /etc/passwd through.
	HTTPFetchTempFilePrefix = "n9e-aiagent-fetch-"
)

// httpFetch issues a GET against a public URL and returns the response body
// as text. Built for skills that need to pull a remote artifact (Prometheus
// rule YAML, Grafana dashboard JSON, etc.) into the agent loop.
//
// SSRF defenses, in order:
//  1. Scheme must be http/https.
//  2. All resolved IPs for the host must be public — checked in safeDialContext
//     so DNS rebinding can't bypass us.
//  3. Redirects are re-validated and capped (httpFetchMaxRedirects).
//  4. Body is capped at httpFetchHardMaxBytes regardless of caller request.
func httpFetch(ctx context.Context, _ *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	rawURL := getArgString(args, "url")
	if rawURL == "" {
		return "", fmt.Errorf("url is required")
	}

	if err := validateFetchURL(rawURL); err != nil {
		return "", err
	}

	maxBytes := getArgInt(args, "max_bytes", httpFetchDefaultMaxBytes)
	if maxBytes > httpFetchHardMaxBytes {
		maxBytes = httpFetchHardMaxBytes
	}

	timeoutSec := getArgInt(args, "timeout_seconds", httpFetchDefaultTimeout)
	if timeoutSec > httpFetchMaxTimeout {
		timeoutSec = httpFetchMaxTimeout
	}

	transport := &http.Transport{
		DialContext:           safeDialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		DisableKeepAlives:     true,
	}
	client := &http.Client{
		Timeout:   time.Duration(timeoutSec) * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= httpFetchMaxRedirects {
				return fmt.Errorf("too many redirects (>%d)", httpFetchMaxRedirects)
			}
			return validateFetchURL(req.URL.String())
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid request: %v", err)
	}
	req.Header.Set("User-Agent", "n9e-aiagent-http-fetch/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http fetch failed: %v", err)
	}
	defer resp.Body.Close()

	// Read one byte beyond the cap so we can detect truncation.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return "", fmt.Errorf("read body failed: %v", err)
	}
	truncated := len(raw) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}

	saveToFile := false
	if v, ok := args["save_to_file"].(bool); ok {
		saveToFile = v
	}

	if saveToFile {
		// Write body to a temp file and return its path instead of the bytes.
		// Lets the LLM hand the path to downstream tools (preview/import) without
		// the YAML ever entering the prompt context — saves tokens on large files
		// and keeps the LLM from "helpfully" reformatting it on the way through.
		f, err := os.CreateTemp("", HTTPFetchTempFilePrefix+"*")
		if err != nil {
			return "", fmt.Errorf("create temp file failed: %v", err)
		}
		path := f.Name()
		if _, err := f.Write(raw); err != nil {
			f.Close()
			os.Remove(path)
			return "", fmt.Errorf("write temp file failed: %v", err)
		}
		if err := f.Close(); err != nil {
			os.Remove(path)
			return "", fmt.Errorf("close temp file failed: %v", err)
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"status_code":  resp.StatusCode,
			"content_type": resp.Header.Get("Content-Type"),
			"size":         len(raw),
			"truncated":    truncated,
			"file_path":    path,
		})
		return string(payload), nil
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"size":         len(raw),
		"truncated":    truncated,
		"body":         string(raw),
	})
	return string(payload), nil
}

// ReadFetchTempFile validates that path was produced by http_fetch (sits in
// os.TempDir() and basename starts with HTTPFetchTempFilePrefix) and reads it
// with the same hard cap as the fetcher itself. Exported so the alert-rule
// tools can accept a `payload_file` arg without redoing the path checks.
func ReadFetchTempFile(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("file_path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid file_path: %v", err)
	}
	tmpDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("resolve tmpdir failed: %v", err)
	}
	dir, base := filepath.Split(abs)
	// Trim trailing slash from dir for the equality check (filepath.Split keeps it).
	cleanDir := filepath.Clean(dir)
	cleanTmp := filepath.Clean(tmpDir)
	if cleanDir != cleanTmp {
		return nil, fmt.Errorf("payload_file must live under %s (got %s)", cleanTmp, cleanDir)
	}
	if !strings.HasPrefix(base, HTTPFetchTempFilePrefix) {
		return nil, fmt.Errorf("payload_file basename must start with %q", HTTPFetchTempFilePrefix)
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("open payload_file: %v", err)
	}
	defer f.Close()
	// Same hard cap as http_fetch — refuses to read a 50 MiB file even if it
	// somehow landed in /tmp with our prefix.
	raw, err := io.ReadAll(io.LimitReader(f, int64(httpFetchHardMaxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read payload_file: %v", err)
	}
	if len(raw) > httpFetchHardMaxBytes {
		return nil, fmt.Errorf("payload_file exceeds %d bytes", httpFetchHardMaxBytes)
	}
	return raw, nil
}

func validateFetchURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (allowed: http, https)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("url has no host")
	}
	return nil
}

// safeDialContext resolves DNS itself and refuses any address that is not a
// routable public address. Dialing the IP we resolved (not the hostname)
// closes the DNS-rebinding window where the name maps to a public IP at
// validate time and a private IP at connect time.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup failed for %s: %v", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	for _, ip := range ips {
		if !isPublicIP(ip.IP) {
			return nil, fmt.Errorf("blocked: %s resolves to non-public address %s", host, ip.IP.String())
		}
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsPrivate() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		// 100.64.0.0/10 — Carrier-Grade NAT.
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return false
		}
		// 192.0.0.0/24 — IETF Protocol Assignments.
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0 {
			return false
		}
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 — TEST-NET.
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return false
		}
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return false
		}
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return false
		}
		// 198.18.0.0/15 — benchmarking.
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return false
		}
		// 240.0.0.0/4 — reserved (includes 255.255.255.255).
		if ip4[0] >= 240 {
			return false
		}
	}
	return true
}
