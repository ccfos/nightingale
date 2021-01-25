package collector

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/toolkits/pkg/logger"
)

var fieldCache sync.Map // map[reflect.Type]structFields

type Field struct {
	Name        string             `json:"name,omitempty"`
	Label       string             `json:"label,omitempty"`
	Default     interface{}        `json:"default,omitempty"`
	Enum        []interface{}      `json:"enum,omitempty"`
	Example     string             `json:"example,omitempty"`
	Format      string             `json:"format,omitempty"`
	Description string             `json:"description,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Items       *Field             `json:"items,omitempty" description:"arrays's items"`
	Type        string             `json:"type,omitempty" description:"boolean,integer,folat,string,array"`
	Ref         string             `json:"$ref,omitempty" description:"name of the struct ref"`
	Fields      []Field            `json:"fields,omitempty" description:"fields of struct type"`
	Definitions map[string][]Field `json:"definitions,omitempty"`

	// list      []Field
	skip  bool `json:"-"`
	index []int
	typ   reflect.Type
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
	current := []Field{}
	next := []Field{{typ: t}}

	// Count of queued names for current level and the next.
	var count, nextCount map[reflect.Type]int

	// Types already visited at an earlier level.
	visited := map[reflect.Type]bool{}

	// Fields found.
	var fields []Field

	for len(next) > 0 {
		current, next = next, current[:0]
		count, nextCount = nextCount, map[reflect.Type]int{}

		for _, f := range current {
			if visited[f.typ] {
				continue
			}
			visited[f.typ] = true

			// Scan f.typ for fields to include.
			for i := 0; i < f.typ.NumField(); i++ {
				sf := f.typ.Field(i)
				isUnexported := sf.PkgPath != ""
				if sf.Anonymous {
					t := sf.Type
					if t.Kind() == reflect.Ptr {
						t = t.Elem()
					}
					if isUnexported && t.Kind() != reflect.Struct {
						// Ignore embedded fields of unexported non-struct types.
						continue
					}
					// Do not ignore embedded fields of unexported struct types
					// since they may have exported fields.
				} else if isUnexported {
					// Ignore unexported non-embedded fields.
					continue
				}

				field := getTagOpt(sf)
				if field.skip {
					continue
				}
				index := make([]int, len(f.index)+1)
				copy(index, f.index)
				index[len(f.index)] = i

				ft := sf.Type
				if ft.Name() == "" && ft.Kind() == reflect.Ptr {
					// Follow pointer.
					ft = ft.Elem()
				}

				fieldType(ft, &field, definitions)

				// Record found field and index sequence.
				if field.Name != "" || !sf.Anonymous || ft.Kind() != reflect.Struct {
					field.index = index
					field.typ = ft

					fields = append(fields, field)
					if count[f.typ] > 1 {
						// If there were multiple instances, add a second,
						// so that the annihilation code will see a duplicate.
						// It only cares about the distinction between 1 or 2,
						// so don't bother generating any more copies.
						fields = append(fields, fields[len(fields)-1])
					}
					continue
				}

				// Record new anonymous struct to explore in next round.
				nextCount[ft]++
				if nextCount[ft] == 1 {
					next = append(next, Field{index: index, typ: ft})
				}

			}
		}
	}

	definitions[t.String()] = fields

	return Field{Fields: fields, Definitions: definitions}
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
	opt.Example = sf.Tag.Get("example")
	opt.Format = sf.Tag.Get("format")
	opt.Description = _s(sf.Tag.Get("description"))
	if s := sf.Tag.Get("enum"); s != "" {
		if err := json.Unmarshal([]byte(s), &opt.Enum); err != nil {
			logger.Warningf("%s.enum %s Unmarshal err %s",
				sf.Name, s, err)
		}
	}
	if s := sf.Tag.Get("default"); s != "" {
		if err := json.Unmarshal([]byte(s), &opt.Default); err != nil {
			logger.Warningf("%s.default %s Unmarshal err %s",
				sf.Name, s, err)
		}
	}

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
