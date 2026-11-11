package dsauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// verifySigV4 re-signs the received request with the given credentials and
// compares the signature, restricted to the headers the client signed.
func verifySigV4(t *testing.T, r *http.Request, body []byte, creds aws.Credentials, region, service string) {
	t.Helper()
	authz := r.Header.Get("Authorization")
	wantScope := fmt.Sprintf("Credential=%s/", creds.AccessKeyID)
	if !strings.HasPrefix(authz, "AWS4-HMAC-SHA256 ") || !strings.Contains(authz, wantScope) {
		t.Fatalf("unexpected Authorization: %q", authz)
	}
	if !strings.Contains(authz, "/"+region+"/"+service+"/aws4_request") {
		t.Fatalf("credential scope missing region/service: %q", authz)
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	if got := r.Header.Get("X-Amz-Content-Sha256"); got != hash {
		t.Fatalf("X-Amz-Content-Sha256 = %q, want %q", got, hash)
	}

	signed := map[string]bool{}
	for _, part := range strings.Split(authz, ", ") {
		if strings.HasPrefix(part, "SignedHeaders=") {
			for _, h := range strings.Split(strings.TrimPrefix(part, "SignedHeaders="), ";") {
				signed[h] = true
			}
		}
	}

	ts, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
	if err != nil {
		t.Fatalf("bad X-Amz-Date: %v", err)
	}

	cp := r.Clone(context.Background())
	cp.URL.Scheme = "http"
	cp.URL.Host = r.Host
	cp.Header = http.Header{}
	for k, v := range r.Header {
		if signed[strings.ToLower(k)] && k != "Authorization" {
			cp.Header[k] = v
		}
	}
	cp.Header.Del("X-Amz-Date")
	cp.Header.Del("X-Amz-Security-Token")
	if err := v4.NewSigner().SignHTTP(context.Background(), creds, cp, hash, service, region, ts); err != nil {
		t.Fatal(err)
	}
	if cp.Header.Get("Authorization") != authz {
		t.Fatalf("signature mismatch\n got: %s\nwant: %s", authz, cp.Header.Get("Authorization"))
	}
}

func TestSigV4StaticKeysGetAndPost(t *testing.T) {
	creds := aws.Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret"}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		verifySigV4(t, r, body, creds, "us-west-2", "aps")
		if r.Method == http.MethodPost && string(body) != "query=up&time=1" {
			t.Fatalf("body not forwarded intact: %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt, err := Wrap(http.DefaultTransport, Config{Type: TypeSigV4, SigV4: &SigV4Config{
		Region: "us-west-2", CredentialType: AWSCredentialKeys, AccessKey: creds.AccessKeyID, SecretKey: creds.SecretAccessKey,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cli := &http.Client{Transport: rt}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/query?query=up%7Bjob%3D%22a%22%7D", nil)
	req.Header.Set("Authorization", "Basic should-be-dropped")
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if req.Header.Get("Authorization") != "Basic should-be-dropped" {
		t.Fatal("caller request was mutated")
	}

	// io.MultiReader hides the length, like the datasource proxy's body.
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/v1/query", io.MultiReader(strings.NewReader("query=up&time=1")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = cli.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("hits = %d", hits)
	}
}

func TestSigV4BodyTooLarge(t *testing.T) {
	rt, err := Wrap(http.DefaultTransport, Config{Type: TypeSigV4, SigV4: &SigV4Config{
		Region: "us-east-1", CredentialType: AWSCredentialKeys, AccessKey: "a", SecretKey: "b",
	}})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:1/", strings.NewReader(strings.Repeat("x", maxSignedBody+1)))
	if _, err := rt.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got %v", err)
	}
}

const assumeRoleResponse = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIATEMP</AccessKeyId>
      <SecretAccessKey>tempsecret</SecretAccessKey>
      <SessionToken>temptoken</SessionToken>
      <Expiration>%s</Expiration>
    </Credentials>
    <AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/q/n9e</Arn><AssumedRoleId>AROA:n9e</AssumedRoleId></AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata><RequestId>1</RequestId></ResponseMetadata>
</AssumeRoleResponse>`

func TestSigV4AssumeRoleWithExternalID(t *testing.T) {
	var stsCalls int32
	stsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&stsCalls, 1)
		_ = r.ParseForm()
		if r.Form.Get("Action") != "AssumeRole" || r.Form.Get("RoleArn") != "arn:aws:iam::123456789012:role/q" ||
			r.Form.Get("ExternalId") != "ext-1" || r.Form.Get("RoleSessionName") != roleSessionName {
			t.Errorf("unexpected sts form: %v", r.Form)
		}
		// the base credentials sign the STS call
		if !strings.Contains(r.Header.Get("Authorization"), "Credential=AKIDBASE/") {
			t.Errorf("sts call not signed by base keys: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintf(w, assumeRoleResponse, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	}))
	defer stsSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Security-Token") != "temptoken" {
			t.Errorf("missing session token")
		}
		verifySigV4(t, r, nil, aws.Credentials{AccessKeyID: "ASIATEMP", SecretAccessKey: "tempsecret", SessionToken: "temptoken"}, "eu-west-1", "es")
	}))
	defer srv.Close()

	rt, err := Wrap(http.DefaultTransport, Config{Type: TypeSigV4, SigV4: &SigV4Config{
		Region: "eu-west-1", Service: "es", CredentialType: AWSCredentialKeys, AccessKey: "AKIDBASE", SecretKey: "base",
		RoleArn: "arn:aws:iam::123456789012:role/q", ExternalId: "ext-1", StsEndpoint: stsSrv.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cli := &http.Client{Transport: rt}
	for i := 0; i < 2; i++ {
		resp, err := cli.Get(srv.URL + "/_search")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if atomic.LoadInt32(&stsCalls) != 1 {
		t.Fatalf("assumed role credentials should be cached, sts calls = %d", stsCalls)
	}
}

func TestSigV4AssumeRoleFailureIsClassified(t *testing.T) {
	stsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `<ErrorResponse><Error><Type>Sender</Type><Code>AccessDenied</Code><Message>not authorized</Message></Error><RequestId>1</RequestId></ErrorResponse>`)
	}))
	defer stsSrv.Close()

	rt, err := Wrap(http.DefaultTransport, Config{Type: TypeSigV4, SigV4: &SigV4Config{
		Region: "us-east-1", CredentialType: AWSCredentialKeys, AccessKey: "a", SecretKey: "b",
		RoleArn: "arn:aws:iam::1:role/x", StsEndpoint: stsSrv.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	_, err = rt.RoundTrip(req)
	if CodeOf(err) != CodeAssumeRoleFailed {
		t.Fatalf("code = %q, err = %v", CodeOf(err), err)
	}
}

func TestOAuth2ClientCredentials(t *testing.T) {
	var tokenCalls int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		_ = r.ParseForm()
		user, pass, _ := r.BasicAuth()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("audience") != "mimir" ||
			r.Form.Get("scope") != "read write" || user != "cid" || pass != "csecret" {
			t.Errorf("unexpected token request: form=%v user=%s", r.Form, user)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok-1","token_type":"bearer","expires_in":3600}`)
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
	}))
	defer srv.Close()

	rt, err := Wrap(nil, Config{Type: TypeOAuth2, OAuth2: &OAuth2Config{
		ClientId: "cid", ClientSecret: "csecret", TokenURL: tokenSrv.URL, Scopes: []string{"read", "write"},
		EndpointParams: map[string]string{"audience": "mimir"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cli := &http.Client{Transport: rt}
	for i := 0; i < 3; i++ {
		resp, err := cli.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if atomic.LoadInt32(&tokenCalls) != 1 {
		t.Fatalf("token should be cached, calls = %d", tokenCalls)
	}
}

func TestOAuth2TokenFailureIsClassified(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_client"}`)
	}))
	defer tokenSrv.Close()

	rt, err := Wrap(nil, Config{Type: TypeOAuth2, OAuth2: &OAuth2Config{ClientId: "c", ClientSecret: "s", TokenURL: tokenSrv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	if _, err := rt.RoundTrip(req); CodeOf(err) != CodeTokenFailed {
		t.Fatalf("code = %q, err = %v", CodeOf(err), err)
	}
}

type fakeAzureCred struct {
	calls  int32
	scopes []string
}

func (f *fakeAzureCred) GetToken(_ context.Context, o policy.TokenRequestOptions) (azcore.AccessToken, error) {
	atomic.AddInt32(&f.calls, 1)
	f.scopes = o.Scopes
	return azcore.AccessToken{Token: "aad-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func TestAzureTokenSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer aad-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
	}))
	defer srv.Close()

	cred := &fakeAzureCred{}
	rt := newTokenRoundTripper(http.DefaultTransport, &azureTokenSource{cred: cred, scope: AzureDefaultScope(AzurePublic)})
	cli := &http.Client{Transport: rt}
	for i := 0; i < 2; i++ {
		resp, err := cli.Get(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if cred.calls != 1 {
		t.Fatalf("token should be cached, calls = %d", cred.calls)
	}
	if len(cred.scopes) != 1 || cred.scopes[0] != "https://prometheus.monitor.azure.com/.default" {
		t.Fatalf("scopes = %v", cred.scopes)
	}
}

func TestAzureCredentialConstruction(t *testing.T) {
	for _, c := range []AzureConfig{
		{Cloud: AzurePublic, CredentialType: AzureCredentialClientSecret, TenantId: "t", ClientId: "c", ClientSecret: "s"},
		{Cloud: AzureChina, CredentialType: AzureCredentialManagedIdentity},
		{Cloud: AzureGovernment, CredentialType: AzureCredentialManagedIdentity, ClientId: "uami"},
	} {
		if _, err := Wrap(nil, Config{Type: TypeAzure, Azure: &c}); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
	}
}

func TestValidateAndPolicy(t *testing.T) {
	defer SetPolicy(Policy{})

	sig := func(s SigV4Config) Config { return Config{Type: TypeSigV4, SigV4: &s} }
	cases := []struct {
		name   string
		policy Policy
		cfg    Config
		ok     bool
	}{
		{"legacy empty", Policy{}, Config{}, true},
		{"unknown type", Policy{}, Config{Type: "kerberos"}, false},
		{"sigv4 default chain", Policy{}, sig(SigV4Config{Region: "us-east-1"}), true},
		{"sigv4 missing region", Policy{}, sig(SigV4Config{}), false},
		{"sigv4 keys missing secret", Policy{}, sig(SigV4Config{Region: "r", CredentialType: AWSCredentialKeys, AccessKey: "a"}), false},
		{"sigv4 service not allowed", Policy{}, sig(SigV4Config{Region: "r", Service: "s3"}), false},
		{"sigv4 service allowed by policy", Policy{SigV4AllowedServices: []string{"s3"}}, sig(SigV4Config{Region: "r", Service: "s3"}), true},
		{"ambient disabled", Policy{DisableAmbientCredentials: true}, sig(SigV4Config{Region: "r"}), false},
		{"ambient disabled keys ok", Policy{DisableAmbientCredentials: true}, sig(SigV4Config{Region: "r", CredentialType: AWSCredentialKeys, AccessKey: "a", SecretKey: "b"}), true},
		{"assume role disabled", Policy{DisableAssumeRole: true}, sig(SigV4Config{Region: "r", RoleArn: "arn:aws:iam::1:role/x"}), false},
		{"bad role arn", Policy{}, sig(SigV4Config{Region: "r", RoleArn: "role/x"}), false},
		{"oauth2 ok", Policy{}, Config{Type: TypeOAuth2, OAuth2: &OAuth2Config{ClientId: "c", ClientSecret: "s", TokenURL: "https://idp/token"}}, true},
		{"oauth2 bad url", Policy{}, Config{Type: TypeOAuth2, OAuth2: &OAuth2Config{ClientId: "c", ClientSecret: "s", TokenURL: "idp/token"}}, false},
		{"azure secret missing", Policy{}, Config{Type: TypeAzure, Azure: &AzureConfig{TenantId: "t"}}, false},
		{"azure mi", Policy{}, Config{Type: TypeAzure, Azure: &AzureConfig{CredentialType: AzureCredentialManagedIdentity}}, true},
		{"azure mi ambient disabled", Policy{DisableAmbientCredentials: true}, Config{Type: TypeAzure, Azure: &AzureConfig{CredentialType: AzureCredentialWorkloadIdentity}}, false},
		{"azure unknown cloud", Policy{}, Config{Type: TypeAzure, Azure: &AzureConfig{Cloud: "Mars", CredentialType: AzureCredentialManagedIdentity}}, false},
	}
	for _, tc := range cases {
		SetPolicy(tc.policy)
		err := Validate(tc.cfg)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}

	// Wrap re-checks the policy so tightening it stops existing datasources.
	SetPolicy(Policy{DisableAmbientCredentials: true})
	if _, err := Wrap(nil, sig(SigV4Config{Region: "r"})); err == nil {
		t.Fatal("Wrap should enforce policy")
	}
}

func TestNormalizeDropsUnrelatedSecrets(t *testing.T) {
	c := Config{
		Type:   TypeSigV4,
		SigV4:  &SigV4Config{Region: "r", AccessKey: "leftover", SecretKey: "leftover"},
		OAuth2: &OAuth2Config{ClientSecret: "leftover"},
	}.Normalize()
	if c.OAuth2 != nil || c.SigV4.AccessKey != "" || c.SigV4.Service != "aps" || c.SigV4.CredentialType != AWSCredentialDefault {
		t.Fatalf("normalize = %+v / %+v", c, c.SigV4)
	}
	if !c.Equal(Config{Type: TypeSigV4, SigV4: &SigV4Config{Region: "r", Service: "aps", CredentialType: AWSCredentialDefault}, Azure: &AzureConfig{}}) {
		t.Fatal("Equal should ignore sub-configs of other types")
	}
}

func TestStatusCode(t *testing.T) {
	cloud := Config{Type: TypeOAuth2, OAuth2: &OAuth2Config{}}
	if StatusCode(cloud, 403) != CodeAccessDenied || StatusCode(cloud, 500) != "" || StatusCode(Config{}, 403) != "" {
		t.Fatal("unexpected classification")
	}
}
