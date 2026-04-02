package sender

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newStaticWebhookTestStats() *astats.Stats {
	return &astats.Stats{
		AlertNotifyTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_static_global_webhook_total"},
			[]string{"channel"},
		),
		AlertNotifyErrorTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_static_global_webhook_error_total"},
			[]string{"channel"},
		),
	}
}

func TestSendStaticGlobalWebhookRecordsNewRequestFailure(t *testing.T) {
	prevClient := staticGlobalWebhookClient
	prevConf := staticGlobalWebhookConf
	defer func() {
		staticGlobalWebhookClient = prevClient
		staticGlobalWebhookConf = prevConf
	}()

	NotifyRecordQueue.RemoveAll()
	defer NotifyRecordQueue.RemoveAll()

	staticGlobalWebhookClient = &http.Client{}
	staticGlobalWebhookConf = aconf.GlobalWebhook{Enable: true, Url: "://bad-url"}

	SendStaticGlobalWebhook(
		ctxpkg.NewContext(context.Background(), nil, true),
		&models.AlertCurEvent{Id: 1, Hash: "event-1"},
		newStaticWebhookTestStats(),
	)

	if got := NotifyRecordQueue.Len(); got != 1 {
		t.Fatalf("expected 1 notify record, got %d", got)
	}

	record, ok := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	if !ok {
		t.Fatalf("expected *models.NotificationRecord in queue")
	}

	if record.Status != models.NotiStatusFailure {
		t.Fatalf("expected failure status, got %d", record.Status)
	}

	if record.Channel != staticGlobalWebhookChannel {
		t.Fatalf("expected channel %q, got %q", staticGlobalWebhookChannel, record.Channel)
	}
}

func TestSendStaticGlobalWebhookRecordsTransportFailure(t *testing.T) {
	prevClient := staticGlobalWebhookClient
	prevConf := staticGlobalWebhookConf
	defer func() {
		staticGlobalWebhookClient = prevClient
		staticGlobalWebhookConf = prevConf
	}()

	NotifyRecordQueue.RemoveAll()
	defer NotifyRecordQueue.RemoveAll()

	staticGlobalWebhookClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("transport boom")
		}),
	}
	staticGlobalWebhookConf = aconf.GlobalWebhook{Enable: true, Url: "http://example.com/webhook"}

	SendStaticGlobalWebhook(
		ctxpkg.NewContext(context.Background(), nil, true),
		&models.AlertCurEvent{Id: 2, Hash: "event-2"},
		newStaticWebhookTestStats(),
	)

	if got := NotifyRecordQueue.Len(); got != 1 {
		t.Fatalf("expected 1 notify record, got %d", got)
	}

	record, ok := NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	if !ok {
		t.Fatalf("expected *models.NotificationRecord in queue")
	}

	if record.Status != models.NotiStatusFailure {
		t.Fatalf("expected failure status, got %d", record.Status)
	}

	if !strings.Contains(record.Details, "transport boom") {
		t.Fatalf("expected transport error details, got %q", record.Details)
	}
}
