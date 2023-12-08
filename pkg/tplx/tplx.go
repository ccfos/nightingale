package tplx

import (
	"bytes"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	templateT "text/template"

	"github.com/toolkits/pkg/logger"
)

// func First(v queryResult) (*sample, error) {
// 	if len(v) > 0 {
// 		return v[0], nil
// 	}
// 	return nil, errors.New("first() called on vector with no elements")
// }

// func Label(label string, s *sample) string {
// 	return s.Labels[label]
// }

// func Value(s *sample) float64 {
// 	return s.Value
// }

// func SafeHtml(text string) template.HTML {
// 	return template.HTML(text)
// }

// func Match(pattern, s string) (bool, error) {
// 	return regexp.MatchString(pattern, s)
// }
// func Title(s string) string {
// 	return strings.Title(s)
// }

// func ToUpper(s string) string {
// 	return strings.ToUpper(s)
// }

// func ToLower(s string) string {
// 	return strings.ToLower(s)
// }

// func GraphLink(expr string) string {
// 	return strutil.GraphLinkForExpression(expr)
// }

// func StripPort(hostPort string) string {
// 	host, _, err := net.SplitHostPort(hostPort)
// 	if err != nil {
// 		return hostPort
// 	}
// 	return host
// }

// func StripDomain(hostPort string) string {
// 	host, port, err := net.SplitHostPort(hostPort)
// 	if err != nil {
// 		host = hostPort
// 	}
// 	ip := net.ParseIP(host)
// 	if ip != nil {
// 		return hostPort
// 	}
// 	host = strings.Split(host, ".")[0]
// 	if port != "" {
// 		return net.JoinHostPort(host, port)
// 	}
// 	return host
// }

// func ToTime(i interface{}) (*time.Time, error) {
// 	v, err := convertToFloat(i)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return floatToTime(v)
// }

// func PathPrefix(externalURL *url.URL) string {
// 	return externalURL.Path
// }

// func ExternalURL(externalURL *url.URL) string {
// 	return externalURL.String()
// }

// func ParseDuration(d string) (float64, error) {
// 	v, err := model.ParseDuration(d)
// 	if err != nil {
// 		return 0, err
// 	}
// 	return float64(time.Duration(v)) / float64(time.Second), nil
// }

// func floatToTime(v float64) (*time.Time, error) {
// 	if math.IsNaN(v) || math.IsInf(v, 0) {
// 		return nil, errNaNOrInf
// 	}
// 	timestamp := v * 1e9
// 	if timestamp > math.MaxInt64 || timestamp < math.MinInt64 {
// 		return nil, fmt.Errorf("%v cannot be represented as a nanoseconds timestamp since it overflows int64", v)
// 	}
// 	t := model.TimeFromUnixNano(int64(timestamp)).Time().UTC()
// 	return &t, nil
// }

// func convertToFloat(i interface{}) (float64, error) {
// 	switch v := i.(type) {
// 	case float64:
// 		return v, nil
// 	case string:
// 		return strconv.ParseFloat(v, 64)
// 	case int:
// 		return float64(v), nil
// 	case uint:
// 		return float64(v), nil
// 	case int64:
// 		return float64(v), nil
// 	case uint64:
// 		return float64(v), nil
// 	default:
// 		return 0, fmt.Errorf("can't convert %T to float", v)
// 	}
// }

var TemplateFuncMap = template.FuncMap{
	"escape":                    url.PathEscape,
	"unescaped":                 Unescaped,
	"urlconvert":                Urlconvert,
	"timeformat":                Timeformat,
	"timestamp":                 Timestamp,
	"args":                      Args,
	"reReplaceAll":              ReReplaceAll,
	"match":                     regexp.MatchString,
	"toUpper":                   strings.ToUpper,
	"toLower":                   strings.ToLower,
	"contains":                  strings.Contains,
	"humanize":                  Humanize,
	"humanize1024":              Humanize1024,
	"humanizeDuration":          HumanizeDuration,
	"humanizeDurationInterface": HumanizeDurationInterface,
	"humanizePercentage":        HumanizePercentage,
	"humanizePercentageH":       HumanizePercentageH,
	"add":                       Add,
	"sub":                       Subtract,
	"mul":                       Multiply,
	"div":                       Divide,
	"now":                       Now,
	"toString":                  ToString,
	"formatDecimal":             FormatDecimal,
	"first":                     First,
	"label":                     Label,
	"value":                     Value,
	"strvalue":                  StrValue,
	"safeHtml":                  SafeHtml,
	"title":                     Title,
	"graphLink":                 GraphLink,
	"tableLink":                 TableLink,
	"sortByLabel":               SortByLabel,
	"stripPort":                 StripPort,
	"stripDomain":               StripDomain,
	"toTime":                    ToTime,
	"pathPrefix":                PathPrefix,
	"externalURL":               ExternalURL,
	"parseDuration":             ParseDuration,
}

// ReplaceTemplateUseHtml replaces variables in a template string with values.
//
// It accepts the following parameters:
//
// - name: The name to use when parsing the template
//
// - templateText: The template string containing variables to replace
//
// - templateData: A struct containing fields to replace the variables
//
// It parses the templateText into a template using template.New and template.Parse.
//
// It executes the parsed template with templateData as the data, writing the result
// to a bytes.Buffer.
//
// Any {{.Field}} variables in templateText are replaced with values from templateData.
//
// If there are any errors parsing or executing the template, they are logged and
// the original templateText is returned.
//
// The rendered template string is returned on success.
//
// Example usage:
//
//	type Data struct {
//	  Name string
//	}
//
//	data := Data{"John"}
//
//	output := ReplaceTemplateUseHtml("mytpl", "Hello {{.Name}}!", data)
func ReplaceTemplateUseHtml(name string, templateText string, templateData any) string {
	tpl, err := template.New(name).Parse(templateText)
	if err != nil {
		logger.Warningf("parse config error: %v", err)
		return templateText
	}
	var body bytes.Buffer
	if err := tpl.Execute(&body, templateData); err != nil {
		logger.Warningf("execute config error: %v", err)
		return templateText
	}
	return body.String()
}

func ReplaceTemplateUseText(name string, templateText string, templateData any) string {
	tpl, err := templateT.New(name).Parse(templateText)
	if err != nil {
		logger.Warningf("text parse config error: %v", err)
		return templateText
	}
	var body bytes.Buffer
	if err := tpl.Execute(&body, templateData); err != nil {
		logger.Warningf("text execute config error: %v", err)
		return templateText
	}
	return body.String()
}
