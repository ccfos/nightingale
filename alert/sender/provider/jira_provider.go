package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type JiraProvider struct{}

func (p *JiraProvider) Ident() string {
	return models.Jira
}

func (p *JiraProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("jira provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("jira provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("jira provider requires URL")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("jira provider requires request body")
	}

	return nil
}

func (p *JiraProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *JiraProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "JIRA", Ident: models.Jira, RequestType: "http", Weight: 18, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:     "https://{JIRA Service Account Email}:{API Token}@api.atlassian.com/ex/jira/{CloudID}/rest/api/3/issue",
					Method:  "POST",
					Headers: map[string]string{"Content-Type": "application/json"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{"fields":{"project":{"key":"{{$params.project_key}}"},"issuetype":{"name":"{{if $event.IsRecovered}}Recovery{{else}}Alert{{end}}"},"summary":"{{$event.RuleName}}","description":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"{{$tpl.content}}"}]}]},"labels":["{{join $event.TagsJSON "\",\""}}", "eventHash={{$event.Hash}}"]}}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "project_key", CName: "Project Key", Type: "string"},
					},
				},
			},
		},
	}
}
