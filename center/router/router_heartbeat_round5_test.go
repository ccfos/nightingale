package router

import (
	"bytes"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type heartbeatReadFailure struct {
	err    error
	closed bool
}

func (b *heartbeatReadFailure) Read([]byte) (int, error) { return 0, b.err }
func (b *heartbeatReadFailure) Close() error {
	b.closed = true
	return nil
}

func TestHandleHeartbeat_RejectsInvalidGzipBeforeUpdatingHostState(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/heartbeat", strings.NewReader("not a gzip stream"))
	c.Request.Header.Set("Content-Encoding", "gzip")

	meta, err := HandleHeartbeat(c, nil, "engine", nil, nil, nil)

	require.ErrorIs(t, err, gzip.ErrHeader)
	require.Empty(t, meta.Hostname)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, gzip.ErrHeader.Error(), w.Body.String())
}

func TestHandleHeartbeat_RejectsMalformedJSONBeforeUpdatingHostState(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/heartbeat", strings.NewReader(`{"hostname":`))

	meta, err := HandleHeartbeat(c, nil, "engine", nil, nil, nil)

	require.ErrorContains(t, err, "unexpected end of JSON input")
	require.Empty(t, meta.Hostname)
}

func TestHandleHeartbeat_GzipPayloadRequiresHostname(t *testing.T) {
	var payload bytes.Buffer
	writer := gzip.NewWriter(&payload)
	_, err := writer.Write([]byte(`{"agent_version":"v1","os":"linux"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/heartbeat", &payload)
	c.Request.Header.Set("Content-Encoding", "gzip")

	meta, err := HandleHeartbeat(c, nil, "engine", nil, nil, nil)

	require.EqualError(t, err, "hostname is required")
	require.Equal(t, "v1", meta.AgentVersion)
	require.Equal(t, "linux", meta.OS)
	require.Empty(t, meta.Hostname)
}

func TestHandleHeartbeat_ReadFailureClosesRequestBody(t *testing.T) {
	body := &heartbeatReadFailure{err: errors.New("body interrupted")}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/heartbeat", nil)
	c.Request.Body = body

	meta, err := HandleHeartbeat(c, nil, "engine", nil, nil, nil)

	require.ErrorIs(t, err, body.err)
	require.Empty(t, meta.Hostname)
	require.True(t, body.closed)
}
