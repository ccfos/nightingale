package utils

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

func TplRender(wfCtx *models.WorkflowContext, content string) (string, error) {
	var defs = []string{
		"{{ $event := .Event }}",
		"{{ $labels := .Event.TagsMap }}",
		"{{ $value := .Event.TriggerValue }}",
		"{{ $inputs := .Inputs }}",
	}
	text := strings.Join(append(defs, content), "")
	tpl, err := template.New("tpl").Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	var body bytes.Buffer
	if err = tpl.Execute(&body, wfCtx); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return strings.TrimSpace(body.String()), nil
}
