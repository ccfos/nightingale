package memsto

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"github.com/toolkits/pkg/container/list"
)

type recordingProvider struct {
	recipients [][]string
	result     *provider.NotifyResult
}

func (*recordingProvider) Ident() string                           { return "test" }
func (*recordingProvider) Check(*models.NotifyChannelConfig) error { return nil }
func (p *recordingProvider) Notify(_ context.Context, r *provider.NotifyRequest) *provider.NotifyResult {
	p.recipients = append(p.recipients, append([]string(nil), r.Sendtos...))
	return p.result
}

func TestProcessNotifyTask_RoutesRecipientsAndRecordsResults(t *testing.T) {
	for _, tc := range []struct {
		name, body, kind string
		contacts         []string
		want             [][]string
		targets          []string
	}{
		{
			name:     "individual",
			body:     "$sendto",
			kind:     "http",
			contacts: []string{"a", "b"},
			want:     [][]string{{"a"}, {"b"}},
			targets:  []string{"a", "b"},
		},
		{
			name:     "batch",
			body:     "$sendtos",
			kind:     "http",
			contacts: []string{"a", "b"},
			want:     [][]string{{"a", "b"}},
			targets:  []string{"a,b"},
		},
		{
			name:    "custom recipient",
			body:    "payload",
			kind:    "http",
			want:    [][]string{nil},
			targets: []string{""},
		},
		{name: "other transport", body: "payload", kind: "script", contacts: []string{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.New("delivery failed")
			p := &recordingProvider{result: &provider.NotifyResult{Response: "response", Err: failure}}
			cache := &NotifyChannelCacheType{ctx: ctx.NewContext(context.Background(), nil, true)}
			events := []*models.AlertCurEvent{{Hash: "event"}}
			var targets []string
			cache.SetNotifyRecordFunc(func(
				_ *ctx.Context,
				got []*models.AlertCurEvent,
				id int64,
				channel, target, resp string,
				err error,
			) {
				require.Equal(t, events, got)
				require.Equal(t, int64(9), id)
				require.Equal(t, "ops", channel)
				require.Contains(t, resp, "response")
				require.ErrorIs(t, err, failure)

				targets = append(targets, target)
			})
			// $sendtos requests one batch; $sendto requests one send per recipient.
			request := &provider.NotifyRequest{
				Config: &models.NotifyChannelConfig{
					Name:          "ops",
					RequestType:   tc.kind,
					RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: tc.body}},
				},
				Sendtos: tc.contacts,
				Events:  events,
			}

			cache.processNotifyTask(&NotifyTask{NotifyRuleId: 9, Provider: p, Request: request})

			require.Equal(t, tc.want, p.recipients)
			require.Equal(t, tc.targets, targets)
			require.Equal(t, tc.contacts, request.Sendtos, "individual sends must not mutate the shared request")
		})
	}
}

func channelResourceCache() (*NotifyChannelCacheType, chan struct{}, chan struct{}) {
	queue := list.NewSafeListLimited(1)
	quit := make(chan struct{})
	smtpQuit := make(chan struct{})
	smtp := make(chan *models.EmailContext)
	channel := &models.NotifyChannelConfig{ID: 1, RequestType: "script", UpdateAt: 10}
	c := &NotifyChannelCacheType{
		channels:      map[int64]*models.NotifyChannelConfig{1: channel},
		channelsQueue: map[int64]*list.SafeListLimited{1: queue},
		queueQuitCh:   map[int64]chan struct{}{1: quit},
		smtpQuitCh:    map[int64]chan struct{}{1: smtpQuit},
		smtpCh:        map[int64]chan *models.EmailContext{1: smtp},
		httpClient:    map[int64]*http.Client{1: {}},
	}
	return c, quit, smtpQuit
}

func TestNotifyChannelCache_Lookups(t *testing.T) {
	c, _, _ := channelResourceCache()
	channel := c.channels[1]
	smtp := c.smtpCh[1]

	require.Same(t, channel, c.Get(1))
	require.Nil(t, c.Get(2))
	require.Equal(t, []int64{1}, c.GetChannelIds())
	require.Equal(t, smtp, c.GetSmtpClient(1))
	require.Nil(t, c.GetSmtpClient(2))
	require.NotNil(t, c.GetHttpClient(1))
	require.Nil(t, c.GetHttpClient(2))
}

func TestEnqueueNotifyTask_RejectsFullOrMissingQueue(t *testing.T) {
	c, _, _ := channelResourceCache()
	channel := c.channels[1]
	queue := c.channelsQueue[1]

	task := &NotifyTask{Request: &provider.NotifyRequest{Config: channel}}

	require.True(t, c.EnqueueNotifyTask(task))
	require.False(t, c.EnqueueNotifyTask(task))
	require.Same(t, task, queue.PopBack())
	missingChannelTask := &NotifyTask{
		Request: &provider.NotifyRequest{
			Config: &models.NotifyChannelConfig{ID: 2},
		},
	}

	require.False(t, c.EnqueueNotifyTask(missingChannelTask))
}

func TestNotifyChannelCache_UnchangedConfigPreservesChannel(t *testing.T) {
	c, _, _ := channelResourceCache()
	channel := c.channels[1]

	c.Set(map[int64]*models.NotifyChannelConfig{1: channel}, 1, 10)

	require.Same(t, channel, c.Get(1))
	require.False(t, c.StatChanged(1, 10))
	require.True(t, c.StatChanged(1, 11))
	require.True(t, c.StatChanged(2, 10))
}

func TestNotifyChannelCache_UpdateStopsOldResources(t *testing.T) {
	c, quit, smtpQuit := channelResourceCache()

	replacement := &models.NotifyChannelConfig{ID: 1, RequestType: "script", UpdateAt: 11}
	c.Set(map[int64]*models.NotifyChannelConfig{1: replacement}, 1, 11)

	require.Same(t, replacement, c.Get(1))
	require.Empty(t, c.channelsQueue)
	require.Empty(t, c.httpClient)
	require.Empty(t, c.smtpCh)

	select {
	case <-quit:
	default:
		t.Fatal("updated channel queue was not stopped")
	}
	select {
	case <-smtpQuit:
	default:
		t.Fatal("updated channel SMTP sender was not stopped")
	}
}

func TestNotifyChannelCache_DeletionRemovesChannel(t *testing.T) {
	// The prior update has already stopped this channel's resources.
	c := &NotifyChannelCacheType{
		channels: map[int64]*models.NotifyChannelConfig{
			1: {ID: 1, RequestType: "script", UpdateAt: 11},
		},
	}

	c.Set(map[int64]*models.NotifyChannelConfig{}, 0, 12)

	require.Empty(t, c.GetChannelIds())
	require.Nil(t, c.Get(1))
}

func TestChannelConfigChanged_MissingConfigRequiresUpdate(t *testing.T) {
	c := &NotifyChannelCacheType{}
	channel := &models.NotifyChannelConfig{ID: 1}

	require.True(t, c.channelConfigChanged(nil, channel))
	require.True(t, c.channelConfigChanged(channel, nil))
}

func TestNeedBatchContacts_NilRequestDoesNotBatch(t *testing.T) {
	c := &NotifyChannelCacheType{}

	require.False(t, c.needBatchContacts(nil))
}
