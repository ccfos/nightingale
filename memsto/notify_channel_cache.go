package memsto

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type NotifyChannelCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	channels map[int64]*models.NotifyChannelConfig // key: channel id

	httpConcurrency map[int64]chan struct{}

	httpClient map[int64]*http.Client
	smtpCh     map[int64]chan *models.EmailContext
	smtpQuitCh map[int64]chan struct{}
}

func NewNotifyChannelCache(ctx *ctx.Context, stats *Stats) *NotifyChannelCacheType {
	ncc := &NotifyChannelCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		channels:        make(map[int64]*models.NotifyChannelConfig),
	}
	ncc.SyncNotifyChannels()
	return ncc
}

func (ncc *NotifyChannelCacheType) Reset() {
	ncc.Lock()
	defer ncc.Unlock()

	ncc.statTotal = -1
	ncc.statLastUpdated = -1
	ncc.channels = make(map[int64]*models.NotifyChannelConfig)
}

func (ncc *NotifyChannelCacheType) StatChanged(total, lastUpdated int64) bool {
	if ncc.statTotal == total && ncc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (ncc *NotifyChannelCacheType) Set(m map[int64]*models.NotifyChannelConfig, httpConcurrency map[int64]chan struct{}, httpClient map[int64]*http.Client,
	smtpCh map[int64]chan *models.EmailContext, quitCh map[int64]chan struct{}, total, lastUpdated int64) {
	ncc.Lock()
	for _, k := range ncc.httpConcurrency {
		close(k)
	}
	ncc.httpConcurrency = httpConcurrency
	ncc.channels = m
	ncc.httpClient = httpClient
	ncc.smtpCh = smtpCh

	for i := range ncc.smtpQuitCh {
		close(ncc.smtpQuitCh[i])
	}

	ncc.smtpQuitCh = quitCh

	ncc.Unlock()

	// only one goroutine used, so no need lock
	ncc.statTotal = total
	ncc.statLastUpdated = lastUpdated
}

func (ncc *NotifyChannelCacheType) Get(channelId int64) *models.NotifyChannelConfig {
	ncc.RLock()
	defer ncc.RUnlock()
	return ncc.channels[channelId]
}

func (ncc *NotifyChannelCacheType) GetHttpClient(channelId int64) *http.Client {
	ncc.RLock()
	defer ncc.RUnlock()
	return ncc.httpClient[channelId]
}

func (ncc *NotifyChannelCacheType) GetSmtpClient(channelId int64) chan *models.EmailContext {
	ncc.RLock()
	defer ncc.RUnlock()
	return ncc.smtpCh[channelId]
}

func (ncc *NotifyChannelCacheType) GetChannelIds() []int64 {
	ncc.RLock()
	defer ncc.RUnlock()

	count := len(ncc.channels)
	list := make([]int64, 0, count)
	for channelId := range ncc.channels {
		list = append(list, channelId)
	}

	return list
}

func (ncc *NotifyChannelCacheType) SyncNotifyChannels() {
	err := ncc.syncNotifyChannels()
	if err != nil {
		fmt.Println("failed to sync notify channels:", err)
	}

	go ncc.loopSyncNotifyChannels()
}

func (ncc *NotifyChannelCacheType) loopSyncNotifyChannels() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := ncc.syncNotifyChannels(); err != nil {
			logger.Warning("failed to sync notify channels:", err)
		}
	}
}

func (ncc *NotifyChannelCacheType) syncNotifyChannels() error {
	start := time.Now()
	stat, err := models.NotifyChannelStatistics(ncc.ctx)
	if err != nil {
		dumper.PutSyncRecord("notify_channels", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec NotifyChannelStatistics")
	}

	if !ncc.StatChanged(stat.Total, stat.LastUpdated) {
		ncc.stats.GaugeCronDuration.WithLabelValues("sync_notify_channels").Set(0)
		ncc.stats.GaugeSyncNumber.WithLabelValues("sync_notify_channels").Set(0)
		dumper.PutSyncRecord("notify_channels", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.NotifyChannelGetsAll(ncc.ctx)
	if err != nil {
		dumper.PutSyncRecord("notify_channels", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec NotifyChannelGetsAll")
	}

	m := make(map[int64]*models.NotifyChannelConfig)
	for i := 0; i < len(lst); i++ {
		m[lst[i].ID] = lst[i]
	}

	httpConcurrency := make(map[int64]chan struct{})
	httpClient := make(map[int64]*http.Client)
	smtpCh := make(map[int64]chan *models.EmailContext)
	quitCh := make(map[int64]chan struct{})

	for i := range lst {
		// todo 优化变更粒度

		switch lst[i].RequestType {
		case "http", "flashduty":
			if lst[i].RequestConfig == nil || lst[i].RequestConfig.HTTPRequestConfig == nil {
				logger.Warningf("notify channel %+v http request config not found", lst[i])
				continue
			}

			cli, _ := models.GetHTTPClient(lst[i])
			httpClient[lst[i].ID] = cli
			httpConcurrency[lst[i].ID] = make(chan struct{}, lst[i].RequestConfig.HTTPRequestConfig.Concurrency)
			for j := 0; j < lst[i].RequestConfig.HTTPRequestConfig.Concurrency; j++ {
				httpConcurrency[lst[i].ID] <- struct{}{}
			}
		case "smtp":
			ch := make(chan *models.EmailContext)
			quit := make(chan struct{})
			go ncc.startEmailSender(lst[i].ID, lst[i].RequestConfig.SMTPRequestConfig, ch, quit)
			smtpCh[lst[i].ID] = ch
			quitCh[lst[i].ID] = quit
		default:
		}
	}

	ncc.Set(m, httpConcurrency, httpClient, smtpCh, quitCh, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	ncc.stats.GaugeCronDuration.WithLabelValues("sync_notify_channels").Set(float64(ms))
	ncc.stats.GaugeSyncNumber.WithLabelValues("sync_notify_channels").Set(float64(len(m)))
	logger.Infof("timer: sync notify channels done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("notify_channels", start.Unix(), ms, len(m), "success")

	return nil
}

func (ncc *NotifyChannelCacheType) startEmailSender(chID int64, smtp *models.SMTPRequestConfig, ch chan *models.EmailContext, quitCh chan struct{}) {
	conf := smtp
	if conf.Host == "" || conf.Port == 0 {
		logger.Warning("SMTP configurations invalid")
		return
	}
	logger.Infof("start email sender... conf.Host:%+v,conf.Port:%+v", conf.Host, conf.Port)

	d := gomail.NewDialer(conf.Host, conf.Port, conf.Username, conf.Password)
	if conf.InsecureSkipVerify {
		d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}

	var s gomail.SendCloser
	var open bool
	var size int
	for {
		select {
		case <-quitCh:
			return
		case m, ok := <-ch:
			if !ok {
				return
			}
			if !open {
				s = ncc.dialSmtp(quitCh, d)
				if s == nil {
					// Indicates that the dialing failed and exited the current goroutine directly,
					// but put the Message back in the mailch
					ch <- m
					return
				}
				open = true
			}
			var err error
			if err = gomail.Send(s, m.Mail); err != nil {
				logger.Errorf("email_sender: failed to send: %s", err)

				// close and retry
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}

				s = ncc.dialSmtp(quitCh, d)
				if s == nil {
					// Indicates that the dialing failed and exited the current goroutine directly,
					// but put the Message back in the mailch
					ch <- m
					return
				}
				open = true

				if err = gomail.Send(s, m.Mail); err != nil {
					logger.Errorf("email_sender: failed to retry send: %s", err)
				}
			} else {
				logger.Infof("email_sender: result=succ subject=%v to=%v",
					m.Mail.GetHeader("Subject"), m.Mail.GetHeader("To"))
			}

			// sender.NotifyRecord(ncc.ctx, m.Events, m.NotifyRuleId, models.Email, strings.Join(m.Mail.GetHeader("To"), ","), "", err)
			size++

			if size >= conf.Batch {
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}
				open = false
				size = 0
			}

		// Close the connection to the SMTP server if no email was sent in
		// the last 30 seconds.
		case <-time.After(30 * time.Second):
			if open {
				if err := s.Close(); err != nil {
					logger.Warningf("email_sender: failed to close smtp connection: %s", err)
				}
				open = false
			}
		}
	}
}

func (ncc *NotifyChannelCacheType) dialSmtp(quitCh chan struct{}, d *gomail.Dialer) gomail.SendCloser {
	for {
		select {
		case <-quitCh:
			// Note that Sendcloser is not obtained below,
			// and the outgoing signal (with configuration changes) exits the current dial
			return nil
		default:
			if s, err := d.Dial(); err != nil {
				logger.Errorf("email_sender: failed to dial smtp: %s", err)
			} else {
				return s
			}
			time.Sleep(time.Second)
		}
	}
}

func (ncc *NotifyChannelCacheType) HttpConcurrencyAdd(channelId int64) bool {
	ncc.RLock()
	defer ncc.RUnlock()
	if _, ok := ncc.httpConcurrency[channelId]; !ok {
		return false
	}
	_, ok := <-ncc.httpConcurrency[channelId]
	return ok
}

func (ncc *NotifyChannelCacheType) HttpConcurrencyDone(channelId int64) {
	ncc.RLock()
	defer ncc.RUnlock()
	if _, ok := ncc.httpConcurrency[channelId]; !ok {
		return
	}
	ncc.httpConcurrency[channelId] <- struct{}{}
}
