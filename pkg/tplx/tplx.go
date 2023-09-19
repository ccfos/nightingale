package tplx

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

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
}

// ReplaceMacroVariables replaces variables in a template string with values.
//
// It takes a template name, the template text, and a struct of macro values.
// It parses the template, executes it with the macro values, and returns the result
// as a bytes.Buffer.
//
// The name parameter is the template name to use when parsing.
//
// The templateText parameter is the template string to process.
//
// The macroValue parameter is a struct that contains fields to replace the
// variables in the template with.
//
// For example:
//
//   type Macro struct {
//     Name string
//     Count int
//   }
//
//   macros := Macro{
//     Name: "John",
//     Count: 123,
//   }
//
//   templateText := "Hello {{.Name}}, your count is {{.Count}}"
//
//   output, err := ReplaceMacroVariables("mytemplate", templateText, macros)
//
// This would replace the {{.Name}} and {{.Count}} variables in the
// template with the values from the macro struct.
//
// It returns the processed template as a bytes.Buffer or an error if there was
// a problem parsing or executing the template.
func ReplaceMacroVariables(name string, templateText string, macroValue any) (*bytes.Buffer, error) {
	tpl, err := template.New(name).Parse(templateText)
	if err != nil {
		return nil, fmt.Errorf("parse config error: %v", err)
	}
	var body bytes.Buffer
	if err := tpl.Execute(&body, macroValue); err != nil {
		return nil, fmt.Errorf("execute config error: %v", err)
	}
	return &body, nil
}

var varRegex = regexp.MustCompile(`{{\s*\.(\w+)\s*}}`)

// ExtractTemplateVariables extracts the variable names from a template string.
//
// It uses a regular expression to find all occurrences of {{.var}} patterns in
// the input string and returns a slice of the extracted variable names.
//
// For example:
//
//	s := `Hello {{.name}}, your score is {{.score}}`
//	vars := ExtractTemplateVariables(s)
//
//	vars would contain:
//	  []string{"name", "score"}
//
// The parameter s is the input string to extract variables from.
//
// The return value is a []string slice containing all extracted variable names.
// It will return an empty slice if no variables are found.
func ExtractTemplateVariables(s string) []string {
	matches := varRegex.FindAllStringSubmatch(s, -1)
	vars := make([]string, len(matches))
	for i, match := range matches {
		vars[i] = match[1]
	}
	return vars
}
