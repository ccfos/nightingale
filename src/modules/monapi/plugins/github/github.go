package github

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins/github/github"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
)

func init() {
	collector.CollectorRegister(NewGitHubCollector()) // for monapi
	i18n.DictRegister(langDict)
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Repositories":                    "代码仓库",
			"List of repositories to monitor": "要监视的代码仓库存列表",
			"Access token":                    "访问令牌",
			"Github API access token. Unauthenticated requests are limited to 60 per hour": "Github 接口的访问令牌. 匿名状态下，每小时请求限制为60",
			"Enterprise base url": "Github 企业版地址",
			"Github API enterprise url. Github Enterprise accounts must specify their base url": "如果使用Github企业版，请配置企业版API地址",
			"HTTP timeout":              "请求超时时间",
			"Timeout for HTTP requests": "http请求超时时间, 单位: 秒",
		},
	}
)

type GitHubCollector struct {
	*collector.BaseCollector
}

func NewGitHubCollector() *GitHubCollector {
	return &GitHubCollector{BaseCollector: collector.NewBaseCollector(
		"github",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &GitHubRule{} },
	)}
}

type GitHubRule struct {
	Repositories      []string `label:"Repositories" json:"repositories,required" example:"didi/nightingale" description:"List of repositories to monitor"`
	AccessToken       string   `label:"Access token" json:"access_token" description:"Github API access token. Unauthenticated requests are limited to 60 per hour"`
	EnterpriseBaseURL string   `label:"Enterprise base url" json:"enterprise_base_url" description:"Github API enterprise url. Github Enterprise accounts must specify their base url"`
	HTTPTimeout       int      `label:"HTTP timeout" json:"http_timeout" default:"5" description:"Timeout for HTTP requests"`
}

func (p *GitHubRule) Validate() error {
	if len(p.Repositories) == 0 || p.Repositories[0] == "" {
		return fmt.Errorf("github.rule.repositories must be set")
	}
	if p.HTTPTimeout == 0 {
		p.HTTPTimeout = 5
	}
	return nil
}

func (p *GitHubRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &github.GitHub{
		Repositories:      p.Repositories,
		AccessToken:       p.AccessToken,
		EnterpriseBaseURL: p.EnterpriseBaseURL,
		HTTPTimeout:       time.Second * time.Duration(p.HTTPTimeout),
	}, nil
}
