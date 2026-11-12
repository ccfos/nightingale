package provider

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func emailRound6Request() *NotifyRequest {
	return &NotifyRequest{
		NotifyRuleId: 17,
		Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{
			SMTPRequestConfig: &models.SMTPRequestConfig{From: "alerts@example.test"},
		}},
		Events: []*models.AlertCurEvent{{Id: 42}}, Sendtos: []string{"ops@example.test", "dev@example.test"},
		TplContent: map[string]interface{}{"subject": "CPU high", "content": "<b>91%</b>"},
		SmtpChan:   make(chan *models.EmailContext, 1),
	}
}

func TestEmailProvider_QueuesRenderedMailAndEventContext(t *testing.T) {
	req := emailRound6Request()

	result := new(EmailProvider).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "queued", result.Response)
	require.Equal(t, "ops@example.test,dev@example.test", result.Target)
	require.Len(t, req.SmtpChan, 1)
	queued := <-req.SmtpChan
	require.Equal(t, req.NotifyRuleId, queued.NotifyRuleId)
	require.Equal(t, req.Events, queued.Events)
	require.Equal(t, req.Sendtos, queued.Mail.GetHeader("To"))
	require.Equal(t, []string{"alerts@example.test"}, queued.Mail.GetHeader("From"))
	var wire bytes.Buffer
	_, err := queued.Mail.WriteTo(&wire)
	require.NoError(t, err)
	message, err := mail.ReadMessage(&wire)
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "CPU high", subject)
	require.Contains(t, message.Header.Get("Content-Type"), "text/html")
	body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	require.NoError(t, err)
	require.Equal(t, "<b>91%</b>", string(body))
}

func TestEmailProvider_RejectsUnavailableSMTPQueue(t *testing.T) {
	req := emailRound6Request()
	req.SmtpChan = nil

	result := new(EmailProvider).Notify(context.Background(), req)

	require.EqualError(t, result.Err, "smtp channel not ready")
	require.Equal(t, "ops@example.test,dev@example.test", result.Target)
}

func TestEmailProvider_CanceledContextInterruptsBlockedEnqueue(t *testing.T) {
	req := emailRound6Request()
	req.SmtpChan = make(chan *models.EmailContext)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := new(EmailProvider).Notify(ctx, req)

	require.ErrorIs(t, result.Err, context.Canceled)
	require.Empty(t, result.Response)
}

func TestEmailProvider_ReportsClosedQueue(t *testing.T) {
	req := emailRound6Request()
	close(req.SmtpChan)

	result := new(EmailProvider).Notify(context.Background(), req)

	require.ErrorContains(t, result.Err, "send on closed smtp channel")
	require.Empty(t, result.Response)
}
