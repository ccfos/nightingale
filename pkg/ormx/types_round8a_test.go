package ormx

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestR8aJSONTypes(t *testing.T) {
	for _, array := range []bool{false, true} {
		name, empty, valid := "object", "{}", `{"key":"value"}`
		if array {
			name, empty, valid = "array", "[]", `[1,"two"]`
		}
		t.Run(name, func(t *testing.T) {
			var obj JSONObj
			var arr JSONArr
			var value interface {
				Scan(interface{}) error
				Value() (driver.Value, error)
				MarshalJSON() ([]byte, error)
				UnmarshalJSON([]byte) error
			}
			if array {
				value = &arr
			} else {
				value = &obj
			}
			b, err := value.MarshalJSON()
			require.NoError(t, err)
			require.JSONEq(t, empty, string(b))
			v, err := value.Value()
			require.NoError(t, err)
			require.Nil(t, v)
			for _, input := range []interface{}{valid, []byte(valid)} {
				require.NoError(t, value.Scan(input))
				b, err = value.MarshalJSON()
				require.NoError(t, err)
				require.JSONEq(t, valid, string(b))
				v, err = value.Value()
				require.NoError(t, err)
				require.JSONEq(t, valid, string(v.([]byte)))
			}
			for _, input := range []interface{}{nil, 12, true} {
				require.ErrorContains(t, value.Scan(input), "Failed to unmarshal JSONB value")
			}
			require.Error(t, value.Scan("{"))
			require.NoError(t, value.UnmarshalJSON([]byte(valid)))
			b, err = json.Marshal(value)
			require.NoError(t, err)
			require.JSONEq(t, valid, string(b))
			require.NoError(t, value.Scan(`"legacy"`))
			b, err = value.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, empty, string(b))
			require.NoError(t, value.Scan("null"))
			b, err = value.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, "null", string(b))
		})
	}
}
