package models

import (
	"encoding/json"
	"fmt"
	"html/template"
	"path"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

type NotifyTpl struct {
	Id      int64  `json:"id"`
	Name    string `json:"name"`
	Channel string `json:"channel"`
	Content string `json:"content"`
}

func (n *NotifyTpl) TableName() string {
	return "notify_tpl"
}

func (n *NotifyTpl) Create(c *ctx.Context) error {
	return Insert(c, n)
}

func (n *NotifyTpl) UpdateContent(c *ctx.Context) error {
	return DB(c).Model(n).Update("content", n.Content).Error
}

func (n *NotifyTpl) Update(c *ctx.Context) error {
	return DB(c).Model(n).Select("name").Updates(n).Error
}

func (n *NotifyTpl) CreateIfNotExists(c *ctx.Context, channel string) error {
	count, err := NotifyTplCountByChannel(c, channel)
	if err != nil {
		return errors.WithMessage(err, "failed to count notify tpls")
	}

	if count != 0 {
		return nil
	}

	err = n.Create(c)
	return err
}

func NotifyTplCountByChannel(c *ctx.Context, channel string) (int64, error) {
	var count int64
	err := DB(c).Model(&NotifyTpl{}).Where("channel=?", channel).Count(&count).Error
	return count, err
}

func NotifyTplGets(c *ctx.Context) ([]*NotifyTpl, error) {
	if !c.IsCenter {
		lst, err := poster.GetByUrls[[]*NotifyTpl](c, "/v1/n9e/notify-tpls")
		return lst, err
	}

	var lst []*NotifyTpl
	err := DB(c).Find(&lst).Error
	return lst, err
}

func ListTpls(c *ctx.Context) (map[string]*template.Template, error) {
	notifyTpls, err := NotifyTplGets(c)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get notify tpls")
	}

	tpls := make(map[string]*template.Template)
	for _, notifyTpl := range notifyTpls {
		var defs = []string{
			"{{$labels := .TagsMap}}",
			"{{$value := .TriggerValue}}",
		}
		text := strings.Join(append(defs, notifyTpl.Content), "")
		tpl, err := template.New(notifyTpl.Channel).Funcs(tplx.TemplateFuncMap).Parse(text)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tpl:%v %v ", notifyTpl, err)
		}

		tpls[notifyTpl.Channel] = tpl
	}
	return tpls, nil
}

func InitNotifyConfig(c *ctx.Context, tplDir string) {
	if !c.IsCenter {
		return
	}

	// init notify channel
	cval, err := ConfigsGet(c, NOTIFYCHANNEL)
	if err != nil {
		logger.Errorf("failed to get notify contact config: %v", err)
		return
	}

	if cval == "" {
		var notifyChannels []NotifyChannel
		channels := []string{Dingtalk, Wecom, Feishu, Mm, Telegram, Email}
		for _, channel := range channels {
			notifyChannels = append(notifyChannels, NotifyChannel{Ident: channel, Name: channel, BuiltIn: true})
		}

		data, _ := json.Marshal(notifyChannels)
		err = ConfigsSet(c, NOTIFYCHANNEL, string(data))
		if err != nil {
			logger.Errorf("failed to set notify contact config: %v", err)
			return
		}
	}

	// init notify contact
	cval, err = ConfigsGet(c, NOTIFYCONTACT)
	if err != nil {
		logger.Errorf("failed to get notify contact config: %v", err)
		return
	}

	if cval == "" {
		var notifyContacts []NotifyContact
		contacts := []string{DingtalkKey, WecomKey, FeishuKey, MmKey, TelegramKey}
		for _, contact := range contacts {
			notifyContacts = append(notifyContacts, NotifyContact{Ident: contact, Name: contact, BuiltIn: true})
		}

		data, _ := json.Marshal(notifyContacts)
		err = ConfigsSet(c, NOTIFYCONTACT, string(data))
		if err != nil {
			logger.Errorf("failed to set notify contact config: %v", err)
			return
		}
	}

	// init notify tpl
	filenames, err := file.FilesUnder(tplDir)
	if err != nil {
		logger.Errorf("failed to get tpl files under %s", tplDir)
		return
	}

	if len(filenames) == 0 {
		logger.Errorf("no tpl files under %s", tplDir)
		return
	}

	tplMap := make(map[string]string)
	for i := 0; i < len(filenames); i++ {
		if strings.HasSuffix(filenames[i], ".tpl") {
			name := strings.TrimSuffix(filenames[i], ".tpl")
			tplpath := path.Join(tplDir, filenames[i])
			content, err := file.ToString(tplpath)
			if err != nil {
				logger.Errorf("failed to read tpl file: %s", filenames[i])
				continue
			}
			tplMap[name] = content
		}
	}

	for channel, content := range tplMap {
		notifyTpl := NotifyTpl{
			Name:    channel,
			Channel: channel,
			Content: content,
		}

		err := notifyTpl.CreateIfNotExists(c, channel)
		if err != nil {
			logger.Warningf("failed to create notify tpls %v", err)
		}
	}
}
