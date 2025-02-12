package memsto

import (
	"crypto/tls"
	"fmt"
	"gopkg.in/gomail.v2"
	"net/http"
	"sync"
	"time"

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

func (ncc *NotifyChannelCacheType) Set(m map[int64]*models.NotifyChannelConfig, httpClient map[int64]*http.Client,
	smtpCh map[int64]chan *models.EmailContext, total, lastUpdated int64) {
	ncc.Lock()
	ncc.channels = m
	ncc.httpClient = httpClient
	ncc.smtpCh = smtpCh
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
		exit(1)
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

	httpClient := make(map[int64]*http.Client)
	smtpCh := make(map[int64]chan *models.EmailContext)

	for i := range lst {
		// todo 优化变更粒度

		switch lst[i].RequestType {
		case "http":
			cli, _ := models.GetHTTPClient(lst[i])
			httpClient[lst[i].ID] = cli
		case "smtp":
			ch := make(chan *models.EmailContext)
			ncc.startEmailSender(lst[i].ID, lst[i].SMTPRequestConfig, ch)
			smtpCh[lst[i].ID] = ch
		default:
		}
	}

	ncc.Set(m, httpClient, smtpCh, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	ncc.stats.GaugeCronDuration.WithLabelValues("sync_notify_channels").Set(float64(ms))
	ncc.stats.GaugeSyncNumber.WithLabelValues("sync_notify_channels").Set(float64(len(m)))
	logger.Infof("timer: sync notify channels done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("notify_channels", start.Unix(), ms, len(m), "success")

	return nil
}

func (ncc *NotifyChannelCacheType) startEmailSender(chID int64, smtp models.SMTPRequestConfig, ch chan *models.EmailContext) {
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
		case m, ok := <-ch:
			if !ok {
				return
			}

			if !open {
				s = ncc.dialSmtp(chID, d)
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

				s = ncc.dialSmtp(chID, d)
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

			//for _, to := range m.mail.GetHeader("To") {
			//	msg := ""
			//	if err == nil {
			//		msg = "ok"
			//	}
			//	NotifyRecord(ctx, m.events, models.Email, to, msg, err)
			//}

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

func (ncc *NotifyChannelCacheType) dialSmtp(chID int64, d *gomail.Dialer) gomail.SendCloser {
	for {
		select {
		case <-ncc.smtpQuitCh[chID]:
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
