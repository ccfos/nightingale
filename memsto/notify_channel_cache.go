package memsto

import (
	// TODO(dingtalkapp): "context" 仅在 Stream runner 启动时使用，钉钉应用不上线先注释。
	// "context"
	// TODO(dingtalkapp): "crypto/sha256" 仅在 Stream 指纹计算中使用，钉钉应用不上线先注释。
	// "crypto/sha256"
	"crypto/tls"
	// TODO(dingtalkapp): "encoding/hex" 仅在 Stream 指纹计算中使用，钉钉应用不上线先注释。
	// "encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"gopkg.in/gomail.v2"

	// TODO(dingtalkapp): "alert/naming" 仅用于 DingTalk Stream 主备选举，钉钉应用不上线先注释。
	// "github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	// TODO(dingtalkapp): pkg/dingtalk/stream 已 build tag 屏蔽，这里的导入一起注释；上线时恢复。
	// dtstream "github.com/ccfos/nightingale/v6/pkg/dingtalk/stream"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

// TODO(dingtalkapp): 钉钉应用本次不上线，Stream reconcile 相关常量先注释。
// const dingtalkStreamReconcileInterval = 10 * time.Second

// NotifyTask 表示一个通知发送任务
type NotifyTask struct {
	NotifyRuleId int64
	Request      *provider.NotifyRequest
	Provider     provider.NotifyChannelProvider
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

	// TODO(dingtalkapp): 钉钉应用本次不上线，Stream 相关字段先注释；上线时恢复下列四个字段。
	// dingtalkLeaderNaming  *naming.Naming
	// dingtalkReconcileOnce sync.Once
	// dingtalkStreamMu      sync.Mutex
	// dingtalkStreamRunners map[string]*dingtalkStreamRunner // key = AppKey (ClientId)
}

// TODO(dingtalkapp): 钉钉应用本次不上线，dingtalkStreamRunner 类型先注释；上线时恢复。
// type dingtalkStreamRunner struct {
// 	stop           func()
// 	cfgFingerprint string
// }

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
		// TODO(dingtalkapp): 钉钉应用本次不上线，Stream runners 初始化先注释。
		// dingtalkStreamRunners: make(map[string]*dingtalkStreamRunner),
	}

	ncc.SyncNotifyChannels()
	return ncc
}

// TODO(dingtalkapp): 钉钉应用本次不上线，loopReconcileDingtalkStreams / reconcileDingtalkStreamsFromCache / SetDingtalkLeaderNaming
// 一起注释。上线时恢复下方整段以及 import 顶部的 naming/dtstream/sha256/hex。
// func (ncc *NotifyChannelCacheType) loopReconcileDingtalkStreams() {
// 	ticker := time.NewTicker(dingtalkStreamReconcileInterval)
// 	defer ticker.Stop()
// 	for range ticker.C {
// 		ncc.reconcileDingtalkStreamsFromCache()
// 	}
// }
//
// func (ncc *NotifyChannelCacheType) reconcileDingtalkStreamsFromCache() {
// 	ncc.RLock()
// 	snapshot := make(map[int64]*models.NotifyChannelConfig, len(ncc.channels))
// 	for id, ch := range ncc.channels {
// 		snapshot[id] = ch
// 	}
// 	ncc.RUnlock()
// 	ncc.reconcileDingtalkStreams(snapshot)
// }

// SetNotifyRecordFunc 设置通知记录回调函数
func (ncc *NotifyChannelCacheType) SetNotifyRecordFunc(fn NotifyRecordFunc) {
	ncc.notifyRecordFunc = fn
}

// TODO(dingtalkapp): 钉钉应用本次不上线，SetDingtalkLeaderNaming 入口先注释；调用方 alert/alert.go 也已注释。
// func (ncc *NotifyChannelCacheType) SetDingtalkLeaderNaming(nm *naming.Naming) {
// 	ncc.dingtalkLeaderNaming = nm
// 	if ncc.ctx == nil || !ncc.ctx.IsCenter || ncc.ctx.DB == nil || nm == nil {
// 		return
// 	}
// 	ncc.reconcileDingtalkStreamsFromCache()
// 	ncc.dingtalkReconcileOnce.Do(func() {
// 		go ncc.loopReconcileDingtalkStreams()
// 	})
// }

func (ncc *NotifyChannelCacheType) StatChanged(total, lastUpdated int64) bool {
	if ncc.statTotal == total && ncc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (ncc *NotifyChannelCacheType) Set(m map[int64]*models.NotifyChannelConfig, total, lastUpdated int64) {
	ncc.Lock()
	// 1. 处理需要删除的通道
	ncc.removeDeletedChannels(m)

	// 2. 处理新增和更新的通道
	ncc.addOrUpdateChannels(m)

	ncc.statTotal = total
	ncc.statLastUpdated = lastUpdated
	ncc.Unlock()

	// TODO(dingtalkapp): 钉钉应用本次不上线，快照同步时不再触发 Stream reconcile。
	// ncc.reconcileDingtalkStreams(m)
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
				logger.Debugf("channel %d config not changed", chID)
				continue
			}
		}

		// 更新通道配置
		ncc.channels[chID] = newChannel

		// HTTP client 不在这里预建。GetHttpClient 首次被调用时按需构造，
		// 这样新增通道类型不必再改 cache 逻辑。
		switch newChannel.RequestType {
		case "http":
			ncc.startHttpChannel(chID, newChannel)
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

	// 丢弃已缓存的 HTTP client，保证配置变更后 GetHttpClient 会按新配置重建
	delete(ncc.httpClient, chID)

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
	// TODO: 默认值与 models.GetHTTPClient 中的 Concurrency==0→5 重复，
	// 后续把 HTTPRequestConfig 的默认值统一抽到 normalize() 里，cache 与发送路径共用。
	concurrency := channel.RequestConfig.HTTPRequestConfig.Concurrency
	if concurrency <= 0 {
		concurrency = 5
	}
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
	// 现在只处理 http 类型，flashduty 保持直接发送
	logger.Debugf("processNotifyTask: task: %+v", task)
	if task.Request.Config.RequestType == "http" {
		if len(task.Request.Sendtos) == 0 || ncc.needBatchContacts(task.Request.Config.RequestConfig.HTTPRequestConfig) {
			start := time.Now()
			resut := task.Provider.Notify(ncc.ctx.Ctx, task.Request)
			resp := fmt.Sprintf("send_time: %s duration: %d ms %s", time.Now().Format("2006-01-02 15:04:05"), time.Since(start).Milliseconds(), resut.Response)
			logger.Infof("http_sendernotify_id: %d, channel_name: %v, event:%s, tplContent:%v, customParams:%v, userInfo:%+v, respBody: %v, err: %v",
				task.NotifyRuleId, task.Request.Config.Name, task.Request.Events[0].Hash, task.Request.TplContent, task.Request.CustomParams, task.Request.Sendtos, resp, resut.Err)

			// 调用通知记录回调函数
			if ncc.notifyRecordFunc != nil {
				ncc.notifyRecordFunc(ncc.ctx, task.Request.Events, task.NotifyRuleId, task.Request.Config.Name, ncc.getSendTarget(task.Request.CustomParams, task.Request.Sendtos), resp, resut.Err)
			}
		} else {
			for i := range task.Request.Sendtos {
				// 单人发送模式下，逐个 sendto 渲染并发送，避免在 Provider 内使用全量 Sendtos 造成重复发送。
				reqCopy := *task.Request
				reqCopy.Sendtos = []string{task.Request.Sendtos[i]}
				start := time.Now()
				result := task.Provider.Notify(ncc.ctx.Ctx, &reqCopy)
				resp := fmt.Sprintf("send_time: %s duration: %d ms %s", time.Now().Format("2006-01-02 15:04:05"), time.Since(start).Milliseconds(), result.Response)
				logger.Infof("http_sender notify_id: %d, channel_name: %v, event:%s, tplContent:%v, customParams:%v, userInfo:%+v, respBody: %v, err: %v",
					task.NotifyRuleId, task.Request.Config.Name, task.Request.Events[0].Hash, task.Request.TplContent, task.Request.CustomParams, task.Request.Sendtos[i], resp, result.Err)

				// 调用通知记录回调函数
				if ncc.notifyRecordFunc != nil {
					ncc.notifyRecordFunc(ncc.ctx, task.Request.Events, task.NotifyRuleId, task.Request.Config.Name, ncc.getSendTarget(task.Request.CustomParams, []string{task.Request.Sendtos[i]}), resp, result.Err)
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

// GetHttpClient 懒加载 HTTP client：
//  1. 命中缓存直接返回（fast path, 仅持 RLock）；
//  2. 未命中时根据 channel 配置现场构造并缓存；
//  3. channel 不存在或构造失败返回 nil，由调用方（provider）判空。
//
// 这样新增通道类型不必再在 cache 里登记「是否需要 HTTP client」，
// 避免 dingtalkapp/feishuapp/wecomapp 那种遗漏登记导致真实告警路径拿到 nil 的问题。
func (ncc *NotifyChannelCacheType) GetHttpClient(channelId int64) *http.Client {
	ncc.RLock()
	if cli, ok := ncc.httpClient[channelId]; ok && cli != nil {
		ncc.RUnlock()
		return cli
	}
	ncc.RUnlock()

	ncc.Lock()
	defer ncc.Unlock()
	if cli, ok := ncc.httpClient[channelId]; ok && cli != nil {
		return cli
	}
	// 在写锁内重新读取 channel 配置，避免 RUnlock->Lock 间隙有
	// 配置更新导致用旧 ch 构造出过期的 HTTP client。
	ch := ncc.channels[channelId]
	if ch == nil || ch.RequestConfig == nil {
		return nil
	}
	cli, err := models.GetHTTPClient(ch)
	if err != nil {
		logger.Warningf("lazy build http client for channel %d (type=%s) failed: %v",
			channelId, ch.RequestType, err)
		return nil
	}
	ncc.httpClient[channelId] = cli
	return cli
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
	queue := ncc.channelsQueue[task.Request.Config.ID]
	ncc.RUnlock()

	if queue == nil {
		logger.Errorf("no queue found for channel %d", task.Request.Config.ID)
		return false
	}

	success := queue.PushFront(task)
	if !success {
		logger.Warningf("failed to enqueue notify task for channel %d, queue is full", task.Request.Config.ID)
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

// TODO(dingtalkapp): 钉钉应用本次不上线，下面 Stream 相关函数一起注释；上线时整段恢复，
// 并同时打开顶部 crypto/sha256、encoding/hex、alert/naming、dtstream 的 import。
/*
func dingtalkStreamCfgFingerprint(appKey, appSecret, proxy string) string {
	h := sha256.Sum256([]byte(appKey + "\n" + appSecret + "\n" + proxy))
	return hex.EncodeToString(h[:])
}

type dingtalkStreamDesired struct {
	appKey, appSecret, proxy string
	fingerprint              string
	channel                  *models.NotifyChannelConfig
}

func desiredDingtalkStreamsByAppKey(snapshot map[int64]*models.NotifyChannelConfig) map[string]dingtalkStreamDesired {
	out := make(map[string]dingtalkStreamDesired)
	for _, ch := range snapshot {
		if ch == nil || ch.RequestType != "dingtalkapp" {
			continue
		}
		if ch.RequestConfig == nil || ch.RequestConfig.DingtalkAppRequestConfig == nil {
			continue
		}
		dc := ch.RequestConfig.DingtalkAppRequestConfig
		if strings.TrimSpace(dc.AppKey) == "" || strings.TrimSpace(dc.AppSecret) == "" {
			continue
		}
		fp := dingtalkStreamCfgFingerprint(dc.AppKey, dc.AppSecret, dc.Proxy)
		if ex, ok := out[dc.AppKey]; ok {
			if ex.fingerprint == fp {
				if ex.channel != nil && ch.ID >= ex.channel.ID {
					continue
				}
			} else {
				prevID := int64(0)
				if ex.channel != nil {
					prevID = ex.channel.ID
				}
				logger.Warningf("dingtalkapp duplicate AppKey %s different credentials, prefer smaller channel id: keeping %d over %d",
					dc.AppKey, prevID, ch.ID)
				if ex.channel != nil && ch.ID > ex.channel.ID {
					continue
				}
			}
		}
		out[dc.AppKey] = dingtalkStreamDesired{
			appKey:      dc.AppKey,
			appSecret:   dc.AppSecret,
			proxy:       dc.Proxy,
			fingerprint: fp,
			channel:     ch,
		}
	}
	return out
}

func (ncc *NotifyChannelCacheType) shutdownAllDingtalkStreams() {
	ncc.dingtalkStreamMu.Lock()
	stops := make([]func(), 0, len(ncc.dingtalkStreamRunners))
	for appKey, r := range ncc.dingtalkStreamRunners {
		if r != nil && r.stop != nil {
			logger.Infof("dingtalk stream runner stopping appKey=%s reason=cache_shutdown", appKey)
			stops = append(stops, r.stop)
		}
	}
	ncc.dingtalkStreamRunners = make(map[string]*dingtalkStreamRunner)
	ncc.dingtalkStreamMu.Unlock()
	for _, s := range stops {
		s()
	}
}

func (ncc *NotifyChannelCacheType) reconcileDingtalkStreams(snapshot map[int64]*models.NotifyChannelConfig) {
	if ncc.ctx == nil || !ncc.ctx.IsCenter || ncc.ctx.DB == nil {
		ncc.shutdownAllDingtalkStreams()
		return
	}

	if ncc.dingtalkLeaderNaming == nil || !ncc.dingtalkLeaderNaming.IamLeader() {
		ncc.shutdownAllDingtalkStreams()
		return
	}

	desired := desiredDingtalkStreamsByAppKey(snapshot)

	ncc.dingtalkStreamMu.Lock()
	kept := make(map[string]*dingtalkStreamRunner)
	var stops []func()
	for appKey, r := range ncc.dingtalkStreamRunners {
		w, ok := desired[appKey]
		if ok && w.fingerprint == r.cfgFingerprint {
			kept[appKey] = r
			continue
		}
		if r != nil && r.stop != nil {
			reason := "channel_removed_or_disabled"
			if ok {
				reason = "channel_config_changed"
			}
			logger.Infof("dingtalk stream runner stopping appKey=%s reason=%s", appKey, reason)
			stops = append(stops, r.stop)
		}
	}
	ncc.dingtalkStreamRunners = kept
	ncc.dingtalkStreamMu.Unlock()

	for _, s := range stops {
		s()
	}

	var starts []dingtalkStreamDesired
	ncc.dingtalkStreamMu.Lock()
	for appKey, w := range desired {
		if _, exists := ncc.dingtalkStreamRunners[appKey]; exists {
			continue
		}
		starts = append(starts, w)
	}
	ncc.dingtalkStreamMu.Unlock()

	for _, w := range starts {
		stop := dtstream.StartRunner(context.Background(), dtstream.RunnerDeps{
			Nctx:          ncc.ctx,
			AppKey:        w.appKey,
			AppSecret:     w.appSecret,
			Proxy:         w.proxy,
			NotifyChannel: w.channel,
		})
		ncc.dingtalkStreamMu.Lock()
		if _, exists := ncc.dingtalkStreamRunners[w.appKey]; exists {
			ncc.dingtalkStreamMu.Unlock()
			stop()
			continue
		}
		ncc.dingtalkStreamRunners[w.appKey] = &dingtalkStreamRunner{stop: stop, cfgFingerprint: w.fingerprint}
		ncc.dingtalkStreamMu.Unlock()
		logger.Infof("dingtalk stream runner started appKey=%s", w.appKey)
	}
}
*/

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
