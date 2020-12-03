package collector

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

var fieldCache sync.Map // map[reflect.Type]structFields

type field struct {
	skip        bool   `json:"-"`
	Name        string `json:"name"`
	Label       string `json:"label"`
	Example     string `json:"example"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
}

func (p field) String() string {
	return fmt.Sprintf("name %s label %v format %s skip %v required %v description %s",
		p.Name, p.Label, p.Type, p.skip, p.Required, p.Description)
}

type structFields struct {
	fields []field
}

func (p structFields) String() string {
	var ret string
	for k, v := range p.fields {
		ret += fmt.Sprintf("%d %s\n", k, v)
	}
	return ret
}

// cachedTypeFields is like typeFields but uses a cache to avoid repeated work.
func cachedTypeFields(t reflect.Type) structFields {
	if f, ok := fieldCache.Load(t); ok {
		return f.(structFields)
	}
	f, _ := fieldCache.LoadOrStore(t, typeFields(t))
	return f.(structFields)
}

func typeFields(t reflect.Type) structFields {
	// Fields found.
	var fields []field

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		isUnexported := sf.PkgPath != ""
		if sf.Anonymous {
			panic("unsupported anonymous field")
		} else if isUnexported {
			// Ignore unexported non-embedded fields.
			continue
		}

		field := getTagOpt(sf)
		if field.skip {
			continue
		}
		ft := sf.Type
		if ft.Name() == "" && ft.Kind() == reflect.Ptr {
			// Follow pointer.
			ft = ft.Elem()
		}

		field.Type = ft.String()

		// Record found field and index sequence.
		if field.Name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {

			fields = append(fields, field)
			continue
		}

		panic("unsupported anonymous, struct field")
	}

	return structFields{fields}
}

// tagOptions is the string following a comma in a struct field's "json"
// tag, or the empty string. It does not include the leading comma.
type tagOptions string

// parseTag splits a struct field's json tag into its name and
// comma-separated options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, tagOptions("")
}

// Contains reports whether a comma-separated list of options
// contains a particular substr flag. substr must be surrounded by a
// string boundary or commas.
func (o tagOptions) Contains(optionName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i >= 0 {
			s, next = s[:i], s[i+1:]
		}
		if s == optionName {
			return true
		}
		s = next
	}
	return false
}

func getTagOpt(sf reflect.StructField) (opt field) {
	if sf.Anonymous {
		return
	}

	tag := sf.Tag.Get("json")
	if tag == "-" {
		opt.skip = true
		return
	}

	name, opts := parseTag(tag)
	if opts.Contains("required") {
		opt.Required = true
	}

	opt.Name = name
	opt.Label = sf.Tag.Get("label")
	opt.Example = sf.Tag.Get("example")
	opt.Description = sf.Tag.Get("description")

	return
}

func isValidTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:<=>?@[]^_{|}~ ", c):
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

func panicType(ft reflect.Type, args ...interface{}) {
	msg := fmt.Sprintf("type field %s %s", ft.PkgPath(), ft.Name())

	if len(args) > 0 {
		panic(fmt.Sprint(args...) + " " + msg)
	}
	panic(msg)
}

func Template(v interface{}) (interface{}, error) {
	rv := reflect.Indirect(reflect.ValueOf(v))
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("invalid argument, must be a struct")
	}

	typeFields := cachedTypeFields(rv.Type())

	return typeFields.fields, nil
}
