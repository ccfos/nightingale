//go:build !linux

package skillgateway

import (
	"bytes"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The non-Linux peer verifier accepts any connection. This finite in-memory
// connection exercises framing without a listener, goroutines, or socket I/O.
type protocolConnRound6 struct {
	input          *strings.Reader
	output         bytes.Buffer
	closed         bool
	readDeadlines  int
	writeDeadlines int
}

func (c *protocolConnRound6) Read(b []byte) (int, error)       { return c.input.Read(b) }
func (c *protocolConnRound6) Write(b []byte) (int, error)      { return c.output.Write(b) }
func (c *protocolConnRound6) Close() error                     { c.closed = true; return nil }
func (c *protocolConnRound6) LocalAddr() net.Addr              { return nil }
func (c *protocolConnRound6) RemoteAddr() net.Addr             { return nil }
func (c *protocolConnRound6) SetDeadline(time.Time) error      { return nil }
func (c *protocolConnRound6) SetReadDeadline(time.Time) error  { c.readDeadlines++; return nil }
func (c *protocolConnRound6) SetWriteDeadline(time.Time) error { c.writeDeadlines++; return nil }

func TestHandleConn_RespondsToEachLineAndClosesAtEOF(t *testing.T) {
	conn := &protocolConnRound6{input: strings.NewReader("invalid\n" + `{"path":"/users"}` + "\n")}
	gateway := testGateway("", nil)

	gateway.handleConn(conn)

	require.True(t, conn.closed)
	lines := strings.Split(strings.TrimSuffix(conn.output.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	var malformed, denied response
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &malformed))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &denied))
	require.False(t, malformed.OK)
	require.Contains(t, malformed.Error, "invalid request json")
	require.False(t, denied.OK)
	require.Contains(t, denied.Error, "blacklisted")
	// One read deadline per request plus the final EOF; one write per response.
	require.Equal(t, 3, conn.readDeadlines)
	require.Equal(t, 2, conn.writeDeadlines)
}
