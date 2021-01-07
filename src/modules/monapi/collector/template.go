package collector

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"
)

var fieldCache sync.Map // map[reflect.Type]structFields

type Field struct {
	skip bool `json:"-"`
	// definitions map[string][]Field `json:"-"`

	Name        string             `json:"name,omitempty"`
	Label       string             `json:"label,omitempty"`
	Default     string             `json:"default,omitempty"`
	Example     string             `json:"example,omitempty"`
	Description string             `json:"description,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Items       *Field             `json:"items,omitempty" description:"arrays's items"`
	Type        string             `json:"type,omitempty" description:"boolean,integer,folat,string,array"`
	Ref         string             `json:"$ref,omitempty" description:"name of the struct ref"`
	Fields      []Field            `json:"fields,omitempty" description:"fields of struct type"`
	Definitions map[string][]Field `json:"definitions,omitempty"`
}

func (p Field) String() string {
	return prettify(p)
}

// cachedTypeContent is like typeFields but uses a cache to avoid repeated work.
func cachedTypeContent(t reflect.Type) Field {
	if f, ok := fieldCache.Load(t); ok {
		return f.(Field)
	}
	f, _ := fieldCache.LoadOrStore(t, typeContent(t))
	return f.(Field)
}

func typeContent(t reflect.Type) Field {
	definitions := map[string][]Field{t.String(): nil}

	ret := Field{
		// definitions: map[string][]Field{
		// 	t.String(): nil,
		// },
	}

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

		fieldType(ft, &field, definitions)

		// Record found field and index sequence.
		if field.Name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
			ret.Fields = append(ret.Fields, field)
			continue
		}

		panic("unsupported anonymous, struct field")
	}

	definitions[t.String()] = ret.Fields

	ret.Definitions = definitions

	return ret
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

func getTagOpt(sf reflect.StructField) (opt Field) {
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
	opt.Label = _s(sf.Tag.Get("label"))
	opt.Default = sf.Tag.Get("default")
	opt.Example = sf.Tag.Get("example")
	opt.Description = _s(sf.Tag.Get("description"))

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

	content := cachedTypeContent(rv.Type())

	return content, nil
}

func prettify(in interface{}) string {
	b, _ := json.MarshalIndent(in, "", "  ")
	return string(b)
}

func fieldType(t reflect.Type, in *Field, definitions map[string][]Field) {
	if t.Name() == "" && t.Kind() == reflect.Ptr {
		// Follow pointer.
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		in.Type = "integer"
	case reflect.Float32, reflect.Float64:
		in.Type = "float"
	case reflect.Bool:
		in.Type = "boolean"
	case reflect.String:
		in.Type = "string"
	case reflect.Struct:
		name := t.String()
		if _, ok := definitions[name]; !ok {
			f := cachedTypeContent(t)
			for k, v := range f.Definitions {
				definitions[k] = v
			}
		}
		in.Ref = t.String()
	case reflect.Slice, reflect.Array:
		t2 := t.Elem()
		if t2.Kind() == reflect.Ptr {
			t2 = t2.Elem()
		}
		if k := t2.Kind(); k == reflect.Int || k == reflect.Int32 || k == reflect.Int64 ||
			k == reflect.Uint || k == reflect.Uint32 || k == reflect.Uint64 ||
			k == reflect.Float32 || k == reflect.Float64 ||
			k == reflect.Bool || k == reflect.String || k == reflect.Struct {
			in.Type = "array"
			in.Items = &Field{}
			fieldType(t2, in.Items, definitions)
		} else {
			panic(fmt.Sprintf("unspport type %s items %s", t.String(), t2.String()))
		}
	default:
		panic(fmt.Sprintf("unspport type %s", t.String()))
		// in.Type = "string"
	}
}
