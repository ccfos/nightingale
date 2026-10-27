package dsauth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// maxSignedBody caps how much of a request body is buffered for hashing. SigV4
// needs the payload hash up front, so the body cannot be streamed; the cap
// keeps the datasource proxy from being used to pin large bodies in memory.
const maxSignedBody = 10 << 20

const roleSessionName = "n9e"

// emptyPayloadHash is hex(sha256("")).
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type sigV4RoundTripper struct {
	base    http.RoundTripper
	creds   aws.CredentialsProvider
	signer  *v4.Signer
	region  string
	service string
	roleArn string
	now     func() time.Time
}

func newSigV4RoundTripper(base http.RoundTripper, c SigV4Config) (http.RoundTripper, error) {
	creds, err := NewAWSCredentialsProvider(context.Background(), c)
	if err != nil {
		return nil, err
	}
	return &sigV4RoundTripper{
		base:    base,
		creds:   creds,
		signer:  v4.NewSigner(),
		region:  c.Region,
		service: c.Service,
		roleArn: c.RoleArn,
		now:     time.Now,
	}, nil
}

// NewAWSCredentialsProvider resolves the credential source of c: the SDK
// default chain or static keys, optionally wrapped by AssumeRole. The result
// is cached and refreshed before expiry.
func NewAWSCredentialsProvider(ctx context.Context, c SigV4Config) (aws.CredentialsProvider, error) {
	opts := []func(*config.LoadOptions) error{config.WithRegion(c.Region)}
	if c.CredentialType == AWSCredentialKeys {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.AccessKey, c.SecretKey, "")))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	if c.RoleArn == "" {
		return awsCfg.Credentials, nil
	}

	stsClient := sts.NewFromConfig(awsCfg, func(o *sts.Options) {
		if c.StsEndpoint != "" {
			o.BaseEndpoint = aws.String(c.StsEndpoint)
		}
	})
	provider := stscreds.NewAssumeRoleProvider(stsClient, c.RoleArn, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = roleSessionName
		if c.ExternalId != "" {
			o.ExternalID = aws.String(c.ExternalId)
		}
	})
	return aws.NewCredentialsCache(provider), nil
}

func (rt *sigV4RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	payloadHash, body, err := hashBody(req)
	if err != nil {
		return nil, err
	}

	// RoundTrippers must not modify the caller's request.
	r := req.Clone(req.Context())
	if body != nil {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}

	// A client-supplied Authorization (e.g. forwarded by the proxy) would be
	// replaced by the signature anyway; drop it so it cannot be signed over.
	r.Header.Del("Authorization")
	// The SDK sets this header in a middleware, not in the signer. aoss rejects
	// requests without it, and aps/es accept it.
	r.Header.Set("X-Amz-Content-Sha256", payloadHash)

	creds, err := rt.creds.Retrieve(r.Context())
	if err != nil {
		if rt.roleArn != "" {
			return nil, newError(CodeAssumeRoleFailed, fmt.Errorf("assume role %s: %w", rt.roleArn, err))
		}
		return nil, newError(CodeNoCredentials, err)
	}

	if err := rt.signer.SignHTTP(r.Context(), creds, r, payloadHash, rt.service, rt.region, rt.now()); err != nil {
		return nil, fmt.Errorf("sigv4 sign: %w", err)
	}
	return rt.base.RoundTrip(r)
}

// hashBody reads the request body (if any) and returns its SHA-256 and bytes.
// A nil byte slice means the request had no body.
func hashBody(req *http.Request) (string, []byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return emptyPayloadHash, nil, nil
	}
	defer req.Body.Close()

	body, err := io.ReadAll(io.LimitReader(req.Body, maxSignedBody+1))
	if err != nil {
		return "", nil, fmt.Errorf("read request body for sigv4: %w", err)
	}
	if len(body) > maxSignedBody {
		return "", nil, fmt.Errorf("request body exceeds %d bytes, too large to sign", maxSignedBody)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), body, nil
}

func (rt *sigV4RoundTripper) CloseIdleConnections() {
	closeIdle(rt.base)
}
