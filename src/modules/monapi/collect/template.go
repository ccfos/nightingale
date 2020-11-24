package collect

import (
	"fmt"
	"reflect"
)

func Template(v interface{}) (interface{}, error) {
	rv := reflect.Indirect(reflect.ValueOf(v))
	if rv.Kind() != reflect.Struct {
		return nil, fmt.Errorf("invalid argument, must be a struct")
	}

	typeFields := cachedTypeFields(rv.Type())

	return typeFields.fields, nil
}
