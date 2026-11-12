package sender

import (
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/stretchr/testify/require"
)

type smtpDelivery struct {
	commands []string
	body     []byte
	err      error
}

// localSMTP handles exactly one SMTP session, with no background retry loop.
func localSMTP(t *testing.T, rejectRecipient bool) (aconf.SMTPConfig, <-chan smtpDelivery) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	result := make(chan smtpDelivery, 1)
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	go func() {
		defer close(done)
		var delivery smtpDelivery
		defer func() { result <- delivery }()
		conn, err := listener.Accept()
		if err != nil {
			delivery.err = err
			return
		}
		defer conn.Close()
		// A deadline bounds failures without making delivery assertions depend on elapsed time.
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		wire := textproto.NewConn(conn)
		if err = wire.PrintfLine("220 localhost ESMTP"); err != nil {
			delivery.err = err
			return
		}
		for {
			line, err := wire.ReadLine()
			if err != nil {
				if err != io.EOF {
					delivery.err = err
				}
				return
			}
			delivery.commands = append(delivery.commands, line)
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				err = wire.PrintfLine("250 localhost")
			case strings.HasPrefix(line, "MAIL FROM:"):
				err = wire.PrintfLine("250 sender accepted")
			case strings.HasPrefix(line, "RCPT TO:"):
				if rejectRecipient {
					err = wire.PrintfLine("550 recipient refused")
				} else {
					err = wire.PrintfLine("250 recipient accepted")
				}
			case line == "DATA":
				err = wire.PrintfLine("354 send message")
				if err == nil {
					delivery.body, err = io.ReadAll(wire.DotReader())
				}
				if err == nil {
					err = wire.PrintfLine("250 queued")
				}
			case line == "QUIT":
				delivery.err = wire.PrintfLine("221 goodbye")
				return
			default:
				delivery.err = fmt.Errorf("unexpected SMTP command %q", line)
				return
			}
			if err != nil {
				delivery.err = err
				return
			}
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	return aconf.SMTPConfig{
		Host: "127.0.0.1", Port: address.Port, From: "n9e@example.test", InsecureSkipVerify: true,
	}, result
}

func TestSendEmail_DeliversHTMLAndEnvelopeToLocalSMTP(t *testing.T) {
	config, deliveries := localSMTP(t, false)

	err := SendEmail("CPU high", "<b>usage=91%</b>", []string{"ops@example.test", "dev@example.test"}, config)

	require.NoError(t, err)
	delivery := <-deliveries
	require.NoError(t, delivery.err)
	require.Contains(t, delivery.commands, "MAIL FROM:<n9e@example.test>")
	require.Contains(t, delivery.commands, "RCPT TO:<ops@example.test>")
	require.Contains(t, delivery.commands, "RCPT TO:<dev@example.test>")
	message, err := mail.ReadMessage(strings.NewReader(string(delivery.body)))
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "CPU high", subject)
	require.Contains(t, message.Header.Get("Content-Type"), "text/html")
	body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	require.NoError(t, err)
	// SMTP DATA terminates the final line; DotReader normalizes it to LF.
	require.Equal(t, "<b>usage=91%</b>\n", string(body))
}

func TestSendEmail_ReportsSMTPRecipientRejection(t *testing.T) {
	config, deliveries := localSMTP(t, true)

	err := SendEmail("CPU high", "body", []string{"ops@example.test"}, config)

	require.ErrorContains(t, err, "email_sender: failed to send:")
	require.ErrorContains(t, err, "550 recipient refused")
	delivery := <-deliveries
	require.NoError(t, delivery.err)
	require.Contains(t, delivery.commands, "RCPT TO:<ops@example.test>")
	require.NotContains(t, delivery.commands, "DATA")
	require.Empty(t, delivery.body)
}
