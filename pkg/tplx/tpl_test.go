package tplx

import (
	"html/template"
	"os"
	"testing"
)

func TestRange(t *testing.T) {
	str := "1234,2234,3234,4234"

	// ",", "@", "", "-",

	tmpl := `{{ manipulateStr . "," "@" "" "-" }}`

	tpl, err := template.New("example").Funcs(TemplateFuncMap).Parse(tmpl)
	if err != nil {
		panic(err)
	}

	err = tpl.Execute(os.Stdout, str)
	if err != nil {
		panic(err)
	}
}
