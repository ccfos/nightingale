package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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

	payload, _ := json.Marshal(map[string]interface{}{
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"size":         len(raw),
		"truncated":    truncated,
		"body":         string(raw),
	})
	return string(payload), nil
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
