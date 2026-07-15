package models

import "testing"

// The runtime always speaks Streamable HTTP to an MCP server's URL, so anything
// without an http/https scheme and a host can never connect. Verify() is the
// single choke point every write entry goes through — lock its contract here.
func TestValidateMCPServerURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https ok", "https://mcp.example.com/mcp", false},
		{"http ok", "http://127.0.0.1:8300/mcp", false},
		{"http with port and query ok", "https://mcp.example.com:8443/mcp?x=1", false},
		{"scheme-less host rejected", "mcp.example.com", true},
		{"scheme-less path rejected", "mcp.example.com/mcp", true},
		{"ftp rejected", "ftp://mcp.example.com/mcp", true},
		{"ws rejected", "ws://mcp.example.com/mcp", true},
		{"scheme without host rejected", "http://", true},
		{"empty rejected", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMCPServerURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateMCPServerURL(%q) err = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

// Verify must reject a bad URL too — it's what the create/update tools and the
// HTTP routes all funnel through.
func TestMCPServerVerifyRejectsBadURL(t *testing.T) {
	s := &MCPServer{Name: "x", URL: "mcp.example.com", Private: 0}
	if err := s.Verify(); err == nil {
		t.Fatal("Verify() accepted a scheme-less url, want error")
	}
	s.URL = "https://mcp.example.com/mcp"
	if err := s.Verify(); err != nil {
		t.Fatalf("Verify() rejected a valid url: %v", err)
	}
}
