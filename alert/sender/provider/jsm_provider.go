package provider

import (
	"context"
	"errors"

	"github.com/ccfos/nightingale/v6/models"
)

type JsmProvider struct{}

func (p *JsmProvider) Ident() string {
	return models.JSMAlert // "jsm_alert"
}

func (p *JsmProvider) Check(config *models.NotifyChannelConfig) error {
	if err := config.ValidateHTTPRequestConfig(); err != nil {
		return err
	}

	httpConfig := config.RequestConfig.HTTPRequestConfig

	if httpConfig.Method != "POST" {
		return errors.New("jsm provider requires POST method")
	}

	if httpConfig.Headers == nil || httpConfig.Headers["Content-Type"] != "application/json" {
		return errors.New("jsm provider requires Content-Type: application/json header")
	}

	if httpConfig.URL == "" {
		return errors.New("jsm provider requires URL")
	}

	if httpConfig.Request.Body == "" {
		return errors.New("jsm provider requires request body")
	}

	return nil
}

func (p *JsmProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	httpConfig := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(httpConfig, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: "todo: todo", Response: resp, Err: err}
}

func (p *JsmProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "JSM Alert", Ident: models.JSMAlert, RequestType: "http", Weight: 17, Enable: true,
			RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL:     `https://api.atlassian.com/jsm/ops/integration/v2/alerts{{if $event.IsRecovered}}/{{$event.Hash}}/close?identifierType=alias{{else}}{{end}}`,
					Method:  "POST",
					Headers: map[string]string{"Content-Type": "application/json", "Authorization": "GenieKey {{$params.api_key}}"},
					Timeout: 10000, Concurrency: 5, RetryTimes: 3, RetryInterval: 100,
					Request: models.RequestDetail{
						Body: `{{if $event.IsRecovered}}{"note":"{{$tpl.content}}","source":"{{$event.Cluster}}"}{{else}}{"message":"{{$event.RuleName}}","description":"{{$tpl.content}}","alias":"{{$event.Hash}}","priority":"P{{$event.Severity}}","tags":[{{range $i, $v := $event.TagsJSON}}{{if $i}},{{end}}"{{$v}}"{{end}}],"details":{{jsonMarshal $event.AnnotationsJSON}},"entity":"{{$event.TargetIdent}}","source":"{{$event.Cluster}}"}{{end}}`,
					},
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "api_key", CName: "API Key", Type: "string"},
					},
				},
			},
		},
	}
}
