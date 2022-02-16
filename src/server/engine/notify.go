package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/didi/nightingale/v5/src/storage"
)

var tpls = make(map[string]*template.Template)

var fns = template.FuncMap{
	"unescaped":  func(str string) interface{} { return template.HTML(str) },
	"urlconvert": func(str string) interface{} { return template.URL(str) },
	"timeformat": func(ts int64, pattern ...string) string {
		defp := "2006-01-02 15:04:05"
		if pattern != nil && len(pattern) > 0 {
			defp = pattern[0]
		}
		return time.Unix(ts, 0).Format(defp)
	},
	"timestamp": func(pattern ...string) string {
		defp := "2006-01-02 15:04:05"
		if pattern != nil && len(pattern) > 0 {
			defp = pattern[0]
		}
		return time.Now().Format(defp)
	},
}

func initTpls() error {
	if config.C.Alerting.TemplatesDir == "" {
		config.C.Alerting.TemplatesDir = path.Join(runner.Cwd, "etc", "template")
	}

	filenames, err := file.FilesUnder(config.C.Alerting.TemplatesDir)
	if err != nil {
		return errors.WithMessage(err, "failed to exec FilesUnder")
	}

	if len(filenames) == 0 {
		return errors.New("no tpl files under " + config.C.Alerting.TemplatesDir)
	}

	tplFiles := make([]string, 0, len(filenames))
	for i := 0; i < len(filenames); i++ {
		if strings.HasSuffix(filenames[i], ".tpl") {
			tplFiles = append(tplFiles, filenames[i])
		}
	}

	if len(tplFiles) == 0 {
		return errors.New("no tpl files under " + config.C.Alerting.TemplatesDir)
	}

	for i := 0; i < len(tplFiles); i++ {
		tplpath := path.Join(config.C.Alerting.TemplatesDir, tplFiles[i])

		tpl, err := template.New(tplFiles[i]).Funcs(fns).ParseFiles(tplpath)
		if err != nil {
			return errors.WithMessage(err, "failed to parse tpl: "+tplpath)
		}

		tpls[tplFiles[i]] = tpl
	}

	return nil
}

type Notice struct {
	Event *models.AlertCurEvent `json:"event"`
	Tpls  map[string]string     `json:"tpls"`
}

func buildStdin(event *models.AlertCurEvent) ([]byte, error) {
	// build notice body with templates
	ntpls := make(map[string]string)
	for filename, tpl := range tpls {
		var body bytes.Buffer
		if err := tpl.Execute(&body, event); err != nil {
			ntpls[filename] = err.Error()
		} else {
			ntpls[filename] = body.String()
		}
	}

	return json.Marshal(Notice{Event: event, Tpls: ntpls})
}

func notify(event *models.AlertCurEvent) {
	logEvent(event, "notify")

	stdin, err := buildStdin(event)
	if err != nil {
		logger.Errorf("event_notify: build stdin failed: %v", err)
		return
	}

	// pub all alerts to redis
	if config.C.Alerting.RedisPub.Enable {
		err = storage.Redis.Publish(context.Background(), config.C.Alerting.RedisPub.ChannelKey, stdin).Err()
		if err != nil {
			logger.Errorf("event_notify: redis publish %s err: %v", config.C.Alerting.RedisPub.ChannelKey, err)
		}
	}

	if config.C.Alerting.GlobalCallback.Enable {
		DoGlobalCallback(event)
	}

	// no notify.py? do nothing
	if config.C.Alerting.NotifyScriptPath == "" {
		return
	}

	callScript(stdin)

	// handle alert subscribes
	subs, has := memsto.AlertSubscribeCache.Get(event.RuleId)
	if has {
		handleSubscribes(*event, subs)
	}

	subs, has = memsto.AlertSubscribeCache.Get(0)
	if has {
		handleSubscribes(*event, subs)
	}
}

func DoGlobalCallback(event *models.AlertCurEvent) {
	conf := config.C.Alerting.GlobalCallback
	if conf.Url == "" {
		return
	}

	bs, err := json.Marshal(event)
	if err != nil {
		return
	}

	bf := bytes.NewBuffer(bs)

	req, err := http.NewRequest("POST", conf.Url, bf)
	if err != nil {
		logger.Warning("DoGlobalCallback failed to new request", err)
		return
	}

	if conf.BasicAuthUser != "" && conf.BasicAuthPass != "" {
		req.SetBasicAuth(conf.BasicAuthUser, conf.BasicAuthPass)
	}

	if len(conf.Headers) > 0 && len(conf.Headers)%2 == 0 {
		for i := 0; i < len(conf.Headers); i += 2 {
			req.Header.Set(conf.Headers[i], conf.Headers[i+1])
		}
	}

	client := http.Client{
		Timeout: conf.TimeoutDuration,
	}

	var resp *http.Response
	resp, err = client.Do(req)
	if err != nil {
		logger.Warning("DoGlobalCallback failed to call url, error: ", err)
		return
	}

	var body []byte
	if resp.Body != nil {
		defer resp.Body.Close()
		body, err = ioutil.ReadAll(resp.Body)
	}

	logger.Debugf("DoGlobalCallback done, url: %s, response code: %d, body: %s", conf.Url, resp.StatusCode, string(body))
}

func handleSubscribes(event models.AlertCurEvent, subs []*models.AlertSubscribe) {
	for i := 0; i < len(subs); i++ {
		handleSubscribe(event, subs[i])
	}
}

func handleSubscribe(event models.AlertCurEvent, sub *models.AlertSubscribe) {
	if !matchTags(event.TagsMap, sub.ITags) {
		return
	}

	if sub.RedefineSeverity == 1 {
		event.Severity = sub.NewSeverity
	}

	if sub.RedefineChannels == 1 {
		event.NotifyChannels = sub.NewChannels
		event.NotifyChannelsJSON = strings.Fields(sub.NewChannels)
	}

	event.NotifyGroups = sub.UserGroupIds
	event.NotifyGroupsJSON = strings.Fields(sub.UserGroupIds)
	if len(event.NotifyGroupsJSON) == 0 {
		return
	}

	logEvent(&event, "subscribe")

	fillUsers(&event)

	stdin, err := buildStdin(&event)
	if err != nil {
		logger.Errorf("event_notify: build stdin failed when handle subscribe: %v", err)
		return
	}

	callScript(stdin)
}

func callScript(stdinBytes []byte) {
	fpath := config.C.Alerting.NotifyScriptPath
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, fpath)
	cmd.Stdin = bytes.NewReader(stdinBytes)

	// combine stdout and stderr
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		logger.Errorf("event_notify: run command err: %v", err)
		return
	}
	logger.Infof("event_notify: command Process pid: %d", cmd.Process.Pid)

	if err := cmd.Wait(); err != nil {
		logger.Errorf("event_notify: timeout and killed process %s", fpath)
		return
	}

	if err := ctx.Err(); err != nil {
		logger.Errorf("event_notify: kill process %s occur error %v", fpath, err)
		return
	}

	logger.Infof("event_notify: exec %s output: %s", fpath, buf.String())
}
