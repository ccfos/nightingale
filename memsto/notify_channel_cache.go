package memsto

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"gopkg.in/gomail.v2"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

// NotifyTask 表示一个通知发送任务
type NotifyTask struct {
	Events        []*models.AlertCurEvent
	NotifyRuleId  int64
	NotifyChannel *models.NotifyChannelConfig
	TplContent    map[string]interface{}
	CustomParams  map[string]string
	Sendtos       []string
}

// NotifyRecordFunc 通知记录函数类型
type NotifyRecordFunc func(ctx *ctx.Context, events []*models.AlertCurEvent, notifyRuleId int64, channelName, target, resp string, err error)

type NotifyChannelCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	channels      map[int64]*models.NotifyChannelConfig // key: channel id
	channelsQueue map[int64]*list.SafeListLimited

	httpClient map[int64]*http.Client
	smtpCh     map[int64]chan *models.EmailContext
	smtpQuitCh map[int64]chan struct{}

	// 队列消费者控制
	queueQuitCh map[int64]chan struct{}

	// 通知记录回调函数
	notifyRecordFunc NotifyRecordFunc
}

func NewNotifyChannelCache(ctx *ctx.Context, stats *Stats) *NotifyChannelCacheType {
	ncc := &NotifyChannelCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		channels:        make(map[int64]*models.NotifyChannelConfig),
		channelsQueue:   make(map[int64]*list.SafeListLimited),
		queueQuitCh:     make(map[int64]chan struct{}),
		httpClient:      make(map[int64]*http.Client),
		smtpCh:          make(map[int64]chan *models.EmailContext),
		smtpQuitCh:      make(map[int64]chan struct{}),
	}

	ncc.SyncNotifyChannels()
	return ncc
}

// SetNotifyRecordFunc 设置通知记录回调函数
func (ncc *NotifyChannelCacheType) SetNotifyRecordFunc(fn NotifyRecordFunc) {
	ncc.notifyRecordFunc = fn
}

func (ncc *NotifyChannelCacheType) StatChanged(total, lastUpdated int64) bool {
	if ncc.statTotal == total && ncc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (ncc *NotifyChannelCacheType) Set(m map[int64]*models.NotifyChannelConfig, total, lastUpdated int64) {
	ncc.Lock()
	defer ncc.Unlock()

	// 1. 处理需要删除的通道
	ncc.removeDeletedChannels(m)

	// 2. 处理新增和更新的通道
	ncc.addOrUpdateChannels(m)

	// only one goroutine used, so no need lock
	ncc.statTotal = total
	ncc.statLastUpdated = lastUpdated
}

// removeDeletedChannels 移除已删除的通道
func (ncc *NotifyChannelCacheType) removeDeletedChannels(newChannels map[int64]*models.NotifyChannelConfig) {
	for chID := range ncc.channels {
		if _, exists := newChannels[chID]; !exists {
			logger.Infof("removing deleted channel %d", chID)

			// 停止消费者协程
			if quitCh, exists := ncc.queueQuitCh[chID]; exists {
				close(quitCh)
				delete(ncc.queueQuitCh, chID)
			}

			// 删除队列
			delete(ncc.channelsQueue, chID)

			// 删除HTTP客户端
			delete(ncc.httpClient, chID)

			// 停止SMTP发送器
			if quitCh, exists := ncc.smtpQuitCh[chID]; exists {
				close(quitCh)
				delete(ncc.smtpQuitCh, chID)
				delete(ncc.smtpCh, chID)
			}

			// 删除通道配置
			delete(ncc.channels, chID)
		}
	}
}

// addOrUpdateChannels 添加或更新通道
func (ncc *NotifyChannelCacheType) addOrUpdateChannels(newChannels map[int64]*models.NotifyChannelConfig) {
	for chID, newChannel := range newChannels {
		oldChannel, exists := ncc.channels[chID]
		if exists {
			if ncc.channelConfigChanged(oldChannel, newChannel) {
				logger.Infof("updating channel %d (new: %t)", chID, !exists)
				ncc.stopChannelResources(chID)
			} else {
				logger.Infof("channel %d config not changed", chID)
				continue
			}
		}

		// 更新通道配置
		ncc.channels[chID] = newChannel

		// 根据类型创建相应的资源
		switch newChannel.RequestType {
		case "http", "flashduty":
			// 创建HTTP客户端
			if newChannel.RequestConfig != nil && newChannel.RequestConfig.HTTPRequestConfig != nil {
				cli, err := models.GetHTTPClient(newChannel)
				if err != nil {
					logger.Warningf("failed to create HTTP client for channel %d: %v", chID, err)
				} else {
					if ncc.httpClient == nil {
						ncc.httpClient = make(map[int64]*http.Client)
					}
					ncc.httpClient[chID] = cli
				}
			}

			// 对于 http 类型，启动队列和消费者
			if newChannel.RequestType == "http" {
				ncc.startHttpChannel(chID, newChannel)
			}
		case "smtp":
			// 创建SMTP发送器
			if newChannel.RequestConfig != nil && newChannel.RequestConfig.SMTPRequestConfig != nil {
				ch := make(chan *models.EmailContext)
				quit := make(chan struct{})
				go ncc.startEmailSender(chID, newChannel.RequestConfig.SMTPRequestConfig, ch, quit)

				if ncc.smtpCh == nil {
					ncc.smtpCh = make(map[int64]chan *models.EmailContext)
				}
				if ncc.smtpQuitCh == nil {
					ncc.smtpQuitCh = make(map[int64]chan struct{})
				}
				ncc.smtpCh[chID] = ch
				ncc.smtpQuitCh[chID] = quit
			}
		}
	}
}

// channelConfigChanged 检查通道配置是否发生变化
func (ncc *NotifyChannelCacheType) channelConfigChanged(oldChannel, newChannel *models.NotifyChannelConfig) bool {
	if oldChannel == nil || newChannel == nil {
		return true
	}

	// check updateat
	if oldChannel.UpdateAt != newChannel.UpdateAt {
		return true
	}

	return false
}

// stopChannelResources 停止通道的相关资源
func (ncc *NotifyChannelCacheType) stopChannelResources(chID int64) {
	// 停止HTTP消费者协程
	if quitCh, exists := ncc.queueQuitCh[chID]; exists {
		close(quitCh)
		delete(ncc.queueQuitCh, chID)
		delete(ncc.channelsQueue, chID)
	}

	// 停止SMTP发送器
	if quitCh, exists := ncc.smtpQuitCh[chID]; exists {
		close(quitCh)
		delete(ncc.smtpQuitCh, chID)
		delete(ncc.smtpCh, chID)
	}
}

// startHttpChannel 启动HTTP通道的队列和消费者
func (ncc *NotifyChannelCacheType) startHttpChannel(chID int64, channel *models.NotifyChannelConfig) {
	if channel.RequestConfig == nil || channel.RequestConfig.HTTPRequestConfig == nil {
		logger.Warningf("notify channel %+v http request config not found", channel)
		return
	}

	// 创建队列
	queue := list.NewSafeListLimited(100000)
	ncc.channelsQueue[chID] = queue

	// 启动消费者协程
	quitCh := make(chan struct{})
	ncc.queueQuitCh[chID] = quitCh

	// 启动指定数量的消费者协程
	concurrency := channel.RequestConfig.HTTPRequestConfig.Concurrency
	for i := 0; i < concurrency; i++ {
		go ncc.startNotifyConsumer(chID, queue, quitCh)
	}

	logger.Debugf("started %d notify consumers for channel %d", concurrency, chID)
}

// 启动通知消费者协程
func (ncc *NotifyChannelCacheType) startNotifyConsumer(channelID int64, queue *list.SafeListLimited, quitCh chan struct{}) {
	logger.Debugf("starting notify consumer for channel %d", channelID)

	for {
		select {
		case <-quitCh:
			logger.Debugf("notify consumer for channel %d stopped", channelID)
			return
		default:
			// 从队列中取出任务
			task := queue.PopBack()
			if task == nil {
				// 队列为空，等待一段时间
				time.Sleep(100 * time.Millisecond)
				continue
			}

			notifyTask, ok := task.(*NotifyTask)
			if !ok {
				logger.Errorf("invalid task type in queue for channel %d", channelID)
				continue
			}

			// 处理通知任务
			ncc.processNotifyTask(notifyTask)
		}
	}
}

// processNotifyTask 处理通知任务（仅处理 http 类型）
func (ncc *NotifyChannelCacheType) processNotifyTask(task *NotifyTask) {
	httpClient := ncc.GetHttpClient(task.NotifyChannel.ID)

	// 现在只处理 http 类型，flashduty 保持直接发送
	if task.NotifyChannel.RequestType == "http" {
		if len(task.Sendtos) == 0 || ncc.needBatchContacts(task.NotifyChannel.RequestConfig.HTTPRequestConfig) {
			start := time.Now()
			resp, err := task.NotifyChannel.SendHTTP(task.Events, task.TplContent, task.CustomParams, task.Sendtos, httpClient)
			resp = fmt.Sprintf("duration: %d ms %s", time.Since(start).Milliseconds(), resp)
			logger.Infof("notify_id: %d, channel_name: %v, event:%+v, tplContent:%v, customParams:%v, userInfo:%+v, respBody: %v, err: %v",
				task.NotifyRuleId, task.NotifyChannel.Name, task.Events[0], task.TplContent, task.CustomParams, task.Sendtos, resp, err)

			// 调用通知记录回调函数
			if ncc.notifyRecordFunc != nil {
				ncc.notifyRecordFunc(ncc.ctx, task.Events, task.NotifyRuleId, task.NotifyChannel.Name, ncc.getSendTarget(task.CustomParams, task.Sendtos), resp, err)
			}
		} else {
			for i := range task.Sendtos {
				start := time.Now()
				resp, err := task.NotifyChannel.SendHTTP(task.Events, task.TplContent, task.CustomParams, []string{task.Sendtos[i]}, httpClient)
				resp = fmt.Sprintf("send_time: %s duration: %d ms %s", time.Now().Format("2006-01-02 15:04:05"), time.Since(start).Milliseconds(), resp)
				logger.Infof("notify_id: %d, channel_name: %v, event:%+v, tplContent:%v, customParams:%v, userInfo:%+v, respBody: %v, err: %v",
					task.NotifyRuleId, task.NotifyChannel.Name, task.Events[0], task.TplContent, task.CustomParams, task.Sendtos[i], resp, err)

				// 调用通知记录回调函数
				if ncc.notifyRecordFunc != nil {
					ncc.notifyRecordFunc(ncc.ctx, task.Events, task.NotifyRuleId, task.NotifyChannel.Name, ncc.getSendTarget(task.CustomParams, []string{task.Sendtos[i]}), resp, err)
				}
			}
		}
	}
}

// 判断是否需要批量发送联系人
func (ncc *NotifyChannelCacheType) needBatchContacts(requestConfig *models.HTTPRequestConfig) bool {
	if requestConfig == nil {
		return false
	}
	b, _ := json.Marshal(requestConfig)
	return strings.Contains(string(b), "$sendtos")
}

// 获取发送目标
func (ncc *NotifyChannelCacheType) getSendTarget(customParams map[string]string, sendtos []string) string {
	if len(customParams) == 0 {
		return strings.Join(sendtos, ",")
	}

	values := make([]string, 0)
	for _, value := range customParams {
		runes := []rune(value)
		if len(runes) <= 4 {
			values = append(values, value)
		} else {
			maskedValue := string(runes[:len(runes)-4]) + "****"
			values = append(values, maskedValue)
		}
	}

	return strings.Join(values, ",")
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

// 新增：将通知任务加入队列
func (ncc *NotifyChannelCacheType) EnqueueNotifyTask(task *NotifyTask) bool {
	ncc.RLock()
	queue := ncc.channelsQueue[task.NotifyChannel.ID]
	ncc.RUnlock()

	if queue == nil {
		logger.Errorf("no queue found for channel %d", task.NotifyChannel.ID)
		return false
	}

	success := queue.PushFront(task)
	if !success {
		logger.Warningf("failed to enqueue notify task for channel %d, queue is full", task.NotifyChannel.ID)
	}

	return success
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

	// 增量更新：只传递通道配置，让增量更新逻辑按需创建资源
	ncc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	ncc.stats.GaugeCronDuration.WithLabelValues("sync_notify_channels").Set(float64(ms))
	ncc.stats.GaugeSyncNumber.WithLabelValues("sync_notify_channels").Set(float64(len(m)))
	dumper.PutSyncRecord("notify_channels", start.Unix(), ms, len(m), "success")

	return nil
}

func (ncc *NotifyChannelCacheType) startEmailSender(chID int64, smtp *models.SMTPRequestConfig, ch chan *models.EmailContext, quitCh chan struct{}) {
	conf := smtp
	if conf.Host == "" || conf.Port == 0 {
		logger.Warning("SMTP configurations invalid")
		return
	}
	logger.Debugf("start email sender... conf.Host:%+v,conf.Port:%+v", conf.Host, conf.Port)

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

			// 记录通知详情
			if ncc.notifyRecordFunc != nil {
				target := strings.Join(m.Mail.GetHeader("To"), ",")
				ncc.notifyRecordFunc(ncc.ctx, m.Events, m.NotifyRuleId, "Email", target, "success", err)
			}
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
