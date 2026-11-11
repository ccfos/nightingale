package oauth2x

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	jsoniter "github.com/json-iterator/go"
	"github.com/toolkits/pkg/logger"
)

// maxTokenResponseSize mirrors the limit golang.org/x/oauth2 applies when it
// reads a token response.
const maxTokenResponseSize = 1 << 20

// tokenResponseTransport adapts a non-standard token endpoint, e.g. one that
// answers {"code":200,"msg":"ok","data":{"access_token":...}}, to the RFC 6749
// §5.1 shape golang.org/x/oauth2 parses. Only successful responses from the
// token endpoint are rewritten; every other request (userinfo, introspection)
// passes through untouched.
type tokenResponseTransport struct {
	base     http.RoundTripper
	tokenURL *url.URL
	prefix   string
}

func newTokenResponseTransport(base http.RoundTripper, tokenAddr, prefix string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	u, err := url.Parse(tokenAddr)
	if err != nil {
		logger.Errorf("oauth2: invalid TokenAddr %q, TokenResponsePrefix is ignored: %s", tokenAddr, err)
		return base
	}
	return &tokenResponseTransport{base: base, tokenURL: u, prefix: prefix}
}

func (t *tokenResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode > 299 ||
		req.URL.Host != t.tokenURL.Host || req.URL.Path != t.tokenURL.Path {
		return resp, err
	}

	body, err := ioutil.ReadAll(io.LimitReader(resp.Body, maxTokenResponseSize))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	body = unwrapTokenResponse(body, t.prefix)
	resp.Body = ioutil.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Del("Content-Length")
	resp.Header.Set("Content-Type", "application/json")
	return resp, nil
}

// unwrapTokenResponse lifts the object at prefix to the top level when it
// carries an access_token. A body that is already standard (top-level
// access_token) is returned as is. Anything else, typically a business error
// such as {"code":30110,"msg":"invalid code"} or {"code":500,"data":{}} served
// with HTTP 200, is turned into an RFC 6749 §5.2 error so the caller gets an
// oauth2.RetrieveError carrying the original body.
func unwrapTokenResponse(body []byte, prefix string) []byte {
	node := prefixNode(body, prefix)
	if node.ValueType() == jsoniter.ObjectValue && node.Get("access_token").ValueType() == jsoniter.StringValue {
		return []byte(node.ToString())
	}
	if jsoniter.Get(body, "access_token").ValueType() == jsoniter.StringValue {
		return body
	}

	errBody, _ := json.Marshal(map[string]string{
		"error":             "invalid_token_response",
		"error_description": truncate(body, 512),
	})
	return errBody
}
