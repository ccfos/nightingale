package ormx

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

type JSONObj json.RawMessage
type JSONArr json.RawMessage

// 实现 sql.Scanner 接口，Scan 将 value 扫描至 Jsonb
func (j *JSONObj) Scan(value interface{}) error {
	// 判断是不是byte类型
	bytes, ok := value.([]byte)
	if !ok {
		// 判断是不是string类型
		strings, ok := value.(string)
		if !ok {
			return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
		}
		// string类型转byte[]
		bytes = []byte(strings)
	}

	result := json.RawMessage{}
	err := json.Unmarshal(bytes, &result)
	*j = JSONObj(result)
	return err
}

// 实现 driver.Valuer 接口，Value 返回 json value
func (j JSONObj) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return json.RawMessage(j).MarshalJSON()
}

func (j *JSONObj) MarshalJSON() ([]byte, error) {
	ret := []byte(*j)
	if len(ret) == 0 {
		return []byte(`{}`), nil
	}
	// not valid json
	if ret[0] == '"' {
		return []byte(`{}`), nil
	}
	return ret, nil
}

func (j *JSONObj) UnmarshalJSON(data []byte) error {
	*j = JSONObj(data)
	return nil
}

// 实现 sql.Scanner 接口，Scan 将 value 扫描至 Jsonb
func (j *JSONArr) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	result := json.RawMessage{}
	err := json.Unmarshal(bytes, &result)
	*j = JSONArr(result)
	return err
}

// 实现 driver.Valuer 接口，Value 返回 json value
func (j JSONArr) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return json.RawMessage(j).MarshalJSON()
}

func (j *JSONArr) MarshalJSON() ([]byte, error) {
	ret := []byte(*j)
	if len(ret) == 0 {
		return []byte(`[]`), nil
	}
	// not valid json
	if ret[0] == '"' {
		return []byte(`[]`), nil
	}
	return ret, nil
}

func (j *JSONArr) UnmarshalJSON(data []byte) error {
	*j = JSONArr(data)
	return nil
}
