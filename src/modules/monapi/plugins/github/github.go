package github

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/influxdata/telegraf"
)

func init() {
	collector.CollectorRegister(NewGitHubCollector()) // for monapi
}

type GitHubCollector struct {
	*collector.BaseCollector
}

func NewGitHubCollector() *GitHubCollector {
	return &GitHubCollector{BaseCollector: collector.NewBaseCollector(
		"github",
		collector.RemoteCategory,
		func() interface{} { return &GitHubRule{} },
	)}
}

type GitHubRule struct {
	Repositories      []string `label:"Repositories" json:"repositories" description:"List of repositories to monitor"`
	AccessToken       string   `label:"Access token" json:"access_token" description:"Github API access token.  Unauthenticated requests are limited to 60 per hour"`
	EnterpriseBaseURL string   `label:"Enterprise base url" json:"enterprise_base_url" description:"Github API enterprise url. Github Enterprise accounts must specify their base url"`
	HTTPTimeout       int      `label:"HTTP timeout" json:"http_timeout" description:"Timeout for HTTP requests"`
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

	return &GitHub{
		Repositories:      p.Repositories,
		AccessToken:       p.AccessToken,
		EnterpriseBaseURL: p.EnterpriseBaseURL,
		HTTPTimeout:       time.Second * time.Duration(p.HTTPTimeout),
	}, nil
}
