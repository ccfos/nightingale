package dsauth

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const tokenRequestTimeout = 30 * time.Second

func newOAuth2RoundTripper(base http.RoundTripper, c OAuth2Config) http.RoundTripper {
	cc := &clientcredentials.Config{
		ClientID:     c.ClientId,
		ClientSecret: c.ClientSecret,
		TokenURL:     c.TokenURL,
		Scopes:       c.Scopes,
	}
	if len(c.EndpointParams) > 0 {
		cc.EndpointParams = url.Values{}
		for k, v := range c.EndpointParams {
			cc.EndpointParams.Set(k, v)
		}
	}

	// The token endpoint reuses the datasource transport so TLS (custom CA,
	// mTLS) and proxy settings apply to it as well.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: base,
		Timeout:   tokenRequestTimeout,
	})
	return newTokenRoundTripper(base, cc.TokenSource(ctx))
}

// tokenRoundTripper injects a bearer token from src. It differs from
// oauth2.Transport only in classifying token errors.
type tokenRoundTripper struct {
	base http.RoundTripper
	src  oauth2.TokenSource
}

func newTokenRoundTripper(base http.RoundTripper, src oauth2.TokenSource) *tokenRoundTripper {
	return &tokenRoundTripper{base: base, src: oauth2.ReuseTokenSource(nil, src)}
}

func (rt *tokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := rt.src.Token()
	if err != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, newError(CodeTokenFailed, err)
	}
	r := req.Clone(req.Context())
	tok.SetAuthHeader(r)
	return rt.base.RoundTrip(r)
}

func (rt *tokenRoundTripper) CloseIdleConnections() {
	closeIdle(rt.base)
}
