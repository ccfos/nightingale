package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/v4/src/common/address"
	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/common/slice"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/config"
	"github.com/didi/nightingale/v4/src/modules/server/cron"
	"github.com/didi/nightingale/v4/src/modules/server/redisc"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

func DoNotify(isUpgrade bool, events ...*models.Event) {
	cnt := len(events)
	if cnt == 0 {
		return
	}

	userIds := events[cnt-1].RecvUserIDs

	prio := fmt.Sprintf("p%v", events[cnt-1].Priority)
	eventType := events[cnt-1].EventType

	hashId := strconv.FormatUint(events[cnt-1].HashId, 10)
	workGroups := events[cnt-1].WorkGroups
	content, mailContent := genContent(isUpgrade, events)
	subject := genSubject(isUpgrade, events)

	if len(workGroups) > 0 {
		go send2Ticket(content, subject, hashId, events[cnt-1].Priority, eventType, workGroups)
	}

	if len(userIds) == 0 {
		return
	}
	users, err := models.UserGetByIds(userIds)
	if err != nil {
		logger.Errorf("notify failed, get user by id failed, events: %+v, err: %v", events, err)
		return
	}

	notifyTypes := config.Config.Monapi.Notify[prio]
	for i := 0; i < len(notifyTypes); i++ {
		switch notifyTypes[i] {
		case "voice":
			if events[0].EventType == models.ALERT {
				tos := []string{}
				for j := 0; j < len(users); j++ {
					tos = append(tos, users[j].Phone)
				}

				send(slice.Set(tos), events[0].Sname, "", "voice")
			}
		case "sms":
			tos := []string{}
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Phone)
			}

			send(slice.Set(tos), content, "", "sms")
		case "mail":
			tos := []string{}
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Email)
			}

			if err := send(slice.Set(tos), mailContent, subject, "mail"); err == nil {
				logger.Infof("sendMail: %+v", events[0])
			}
		case "im":
			tos := []string{}
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Im)
			}

			send(slice.Set(tos), content, "", "im")
		default:
			logger.Errorf("not support %s to send notify, events: %+v", notifyTypes[i], events)
		}
	}
}

func genContent(isUpgrade bool, events []*models.Event) (string, string) {
	cnt := len(events)
	if cnt == 0 {
		return "", ""
	}

	cfg := config.Config.Monapi

	metricList := []string{}
	detail, err := events[cnt-1].GetEventDetail()
	if err != nil {
		logger.Errorf("get event detail failed, event: %+v, err: %v", events[cnt-1], err)
	} else {
		for i := 0; i < len(detail); i++ {
			metricList = append(metricList, detail[i].Metric)
		}
	}

	resources := getResources(events)

	metric := strings.Join(metricList, ",")

	status := genStatus(events)
	sname := events[cnt-1].Sname
	endpoint := genEndpoint(events)
	name, note := genNameAndNoteByResources(resources)
	tags := genTags(events)
	value := events[cnt-1].Value
	info := events[cnt-1].Info
	etime := genEtime(events)
	slink := fmt.Sprintf(cfg.Link.Stra, events[cnt-1].Sid)
	elink := fmt.Sprintf(cfg.Link.Event, events[cnt-1].Id)
	clink := ""
	curNodePath := events[cnt-1].CurNodePath

	if events[0].EventType == models.ALERT {
		clink = genClaimLink(events)
	}

	smsContent := ""
	mailContent := ""

	// 获取设备挂载的节点，放到告警信息里发出来，这样可以方便知道设备是属于哪个服务的
	bindings, err := HostBindingsForMon(getEndpoint(events))
	if err != nil {
		bindings = []string{err.Error()}
	}

	isAlert := false
	hasClaim := false
	isMachineDep := false
	if events[0].EventType == models.ALERT {
		isAlert = true
	}

	if clink != "" {
		hasClaim = true
	}

	if events[0].Category == 1 {
		isMachineDep = true
	}

	values := map[string]interface{}{
		"IsAlert":      isAlert,
		"IsMachineDep": isMachineDep,
		"Status":       status,
		"Sname":        sname,
		"Endpoint":     endpoint,
		"Name":         name,
		"Note":         note,
		"CurNodePath":  curNodePath,
		"Metric":       metric,
		"Tags":         tags,
		"Value":        value,
		"Info":         info,
		"Etime":        etime,
		"Elink":        elink,
		"Slink":        slink,
		"HasClaim":     hasClaim,
		"Clink":        clink,
		"IsUpgrade":    isUpgrade,
		"Bindings":     bindings,
	}

	// 生成告警邮件
	fp := path.Join(file.SelfDir(), "etc", "mail.tpl")
	t, err := template.ParseFiles(fp)
	if err != nil {
		logger.Errorf("InternalServerError: cannot parse %s %v", fp, err)
		mailContent = fmt.Sprintf("InternalServerError: cannot parse %s %v", fp, err)
	} else {
		var body bytes.Buffer
		err = t.Execute(&body, values)
		if err != nil {
			logger.Errorf("InternalServerError: %v", err)
			mailContent = fmt.Sprintf("InternalServerError: %v", err)
		} else {
			mailContent += body.String()
		}
	}

	// 生成告警短信，短信和IM复用一个内容模板
	fp = path.Join(file.SelfDir(), "etc", "sms.tpl")
	t, err = template.New("sms.tpl").Funcs(template.FuncMap{
		"unescaped":  func(str string) interface{} { return template.HTML(str) },
		"urlconvert": func(str string) interface{} { return template.URL(str) },
	}).ParseFiles(fp)
	if err != nil {
		logger.Errorf("InternalServerError: cannot parse %s %v", fp, err)
		smsContent = fmt.Sprintf("InternalServerError: cannot parse %s %v", fp, err)
	} else {
		var body bytes.Buffer
		err = t.Execute(&body, values)
		if err != nil {
			logger.Errorf("InternalServerError: %v", err)
			smsContent = fmt.Sprintf("InternalServerError: %v", err)
		} else {
			smsContent += body.String()
		}
	}

	return smsContent, mailContent
}

func genClaimLink(events []*models.Event) string {
	for i := 0; i < len(events); i++ {
		eventCur, err := models.EventCurGet("hashid", events[i].HashId)
		if err != nil {
			logger.Errorf("get event_cur failed, err: %v, event: %+v", err, events[i])
			continue
		}

		if eventCur == nil {
			continue
		}

		return fmt.Sprintf(config.Config.Monapi.Link.Claim, eventCur.Id)
	}
	return ""
}

func genSubject(isUpgrade bool, events []*models.Event) string {
	cnt := len(events)

	subject := ""
	if isUpgrade {
		subject = "[报警已升级]" + subject
	}

	if cnt > 1 {
		subject += fmt.Sprintf("[P%d 聚合%s]%s", events[cnt-1].Priority, models.EventTypeMap[events[cnt-1].EventType], events[cnt-1].Sname)
	} else {
		subject += fmt.Sprintf("[P%d %s]%s", events[cnt-1].Priority, models.EventTypeMap[events[cnt-1].EventType], events[cnt-1].Sname)
	}

	return subject + " - " + genEndpoint(events)
}

func genStatus(events []*models.Event) string {
	cnt := len(events)
	status := fmt.Sprintf("P%d %s", events[cnt-1].Priority, models.EventTypeMap[events[cnt-1].EventType])

	if cnt > 1 {
		status += "（聚合）"
	}

	return status
}

func HostBindingsForMon(endpointList []string) ([]string, error) {
	var list []string
	resouceIds, err := models.ResourceIdsByIdents(endpointList)
	if err != nil {
		return list, err
	}

	nodeIds, err := models.NodeIdsGetByResIds(resouceIds)
	if err != nil {
		return list, err
	}

	for _, id := range nodeIds {
		node, err := models.NodeGet("id=?", id)
		if err != nil {
			return list, err
		}

		if node == nil {
			continue
		}

		list = append(list, node.Path)
	}
	return list, nil
}

func getResources(events []*models.Event) []models.Resource {
	idents := []string{}
	for i := 0; i < len(events); i++ {
		idents = append(idents, events[i].Endpoint)
	}
	resources, err := models.ResourcesByIdents(idents)
	if err != nil {
		logger.Errorf("get resources by idents failed : %v", err)
	}
	return resources
}

func genNameAndNoteByResources(resources []models.Resource) (name, note string) {
	names := []string{}
	notes := []string{}
	for i := 0; i < len(resources); i++ {
		names = append(names, resources[i].Name)
		notes = append(notes, resources[i].Note)
	}
	names = slice.Set(names)
	notes = slice.Set(notes)

	if len(resources) == 1 {
		if len(names) > 0 {
			name = names[0]
		}
		if len(notes) > 0 {
			note = notes[0]
		}
		return
	}
	name = fmt.Sprintf("%s（%v）", strings.Join(names, ","), len(names))
	note = fmt.Sprintf("%s（%v）", strings.Join(notes, ","), len(notes))
	return
}

func getEndpoint(events []*models.Event) []string {
	endpointList := []string{}
	for i := 0; i < len(events); i++ {
		endpointList = append(endpointList, events[i].Endpoint)
	}

	endpointList = slice.Set(endpointList)
	return endpointList
}

func genEndpoint(events []*models.Event) string {
	endpointList := []string{}
	for i := 0; i < len(events); i++ {
		endpointList = append(endpointList, events[i].Endpoint)
	}

	endpointList = slice.Set(endpointList)

	if len(endpointList) == 1 {
		return endpointList[0]
	}

	return fmt.Sprintf("%s（%v）", strings.Join(endpointList, ","), len(endpointList))
}

func genTags(events []*models.Event) string {
	tagsMap := make(map[string][]string)
	for i := 0; i < len(events); i++ {
		detail, err := events[i].GetEventDetail()
		if err != nil {
			continue
		}
		if len(detail) > 0 {
			for k, v := range detail[0].Tags {
				if !slice.InSlice(v, tagsMap[k]) {
					tagsMap[k] = append(tagsMap[k], v)
				}
			}
		}
	}

	tagsList := []string{}
	for k, v := range tagsMap {
		valueString := strings.Join(v, ",")
		if len(v) > 1 {
			valueString = "[" + valueString + "]"
		}
		tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, valueString))
	}

	return strings.Join(tagsList, ",")
}

func genEtime(events []*models.Event) string {
	if len(events) == 1 {
		return models.ParseEtime(events[0].Etime)
	}

	stime := events[0].Etime
	etime := events[0].Etime

	for i := 1; i < len(events); i++ {
		if events[i].Etime < stime {
			stime = events[i].Etime
		}

		if events[i].Etime > etime {
			etime = events[i].Etime
		}
	}

	if stime == etime {
		return models.ParseEtime(stime)
	}

	return models.ParseEtime(stime) + "~" + models.ParseEtime(etime)
}

func send(tos []string, content, subject, notifyType string) error {
	var message dataobj.Message

	if tos == nil || len(tos) == 0 {
		return fmt.Errorf("tos is empty")
	}

	message.Content = strings.TrimSpace(content)
	if message.Content == "" {
		return fmt.Errorf("content is blank")
	}

	if notifyType == "mail" {
		message.Subject = strings.TrimSpace(message.Subject)
		if message.Subject == "" {
			return fmt.Errorf("subject is blank")
		}
	}

	switch notifyType {
	case "voice":
		redisc.Write(&message, cron.VOICE_QUEUE_NAME)
	case "sms":
		redisc.Write(&message, cron.SMS_QUEUE_NAME)
	case "mail":
		redisc.Write(&message, cron.MAIL_QUEUE_NAME)
	case "im":
		redisc.Write(&message, cron.IM_QUEUE_NAME)
	}

	return nil
}

type TicketInfo struct {
	Title         string `json:"title"`
	Typ           string `json:"typ"`
	MonitorTrace  string `json:"monitorTrace"`
	Level         int    `json:"level"`
	Content       string `json:"content"`
	MultiQueueIds []int  `json:"multiQueueIds"`
	AlertType     int    `json:"alertType"`
}

type TicketReq struct {
	Info TicketInfo `json:"ticketInfo"`
}

func send2Ticket(content, subject, hashId string, prio int, eventType string, workGroups []int) {
	if !config.Config.Monapi.TicketEnabled {
		return
	}

	addrs := address.GetHTTPAddresses("ticket")
	perm := rand.Perm(len(addrs))

	for i := range perm {
		url := fmt.Sprintf("%s/v1/ticket/monitor/event?systemName=monitor", addrs[perm[i]])
		if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
			url = "http://" + url
		}

		alertType := 2
		if eventType == "alert" {
			alertType = 1
		}

		info := TicketInfo{
			Title:         subject,
			Typ:           "extern_sync",
			MonitorTrace:  hashId,
			Level:         prio,
			Content:       content,
			MultiQueueIds: workGroups,
			AlertType:     alertType,
		}

		req := TicketReq{
			Info: info,
		}

		res, code, err := httplib.PostJSON(url, time.Second*5, req, map[string]string{"X-Srv-Token": "ticket-builtin-token"})
		if err != nil {
			logger.Errorf("call ticket api failed, server: %v, data: %+v, err: %v, resp:%v, status code:%d", url, req, err, string(res), code)
			return
		}

		if code != 200 {
			logger.Errorf("call ticket api failed, server: %v, data: %+v, resp:%v, code:%d", url, req, string(res), code)
			return
		}

		if err == nil {
			break
		}
	}

	return
}
