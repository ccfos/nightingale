package tplx

import (
	"html/template"
	"testing"
)

func TestBatchContactJsonMarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "整数切片",
			input:    []int{13800138001, 13800138002, 13800138003},
			expected: `["13800138001","13800138002","13800138003"]`,
		},
		{
			name:     "字符串切片",
			input:    []string{"a", "b", "c"},
			expected: `["a","b","c"]`,
		},
		{
			name:     "接口切片",
			input:    []interface{}{1, "b", 3.14},
			expected: `["1","b","3.14"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BatchContactsJsonMarshal(tt.input)
			if result != template.HTML(tt.expected) {
				t.Errorf("期望得到 %v，实际得到 %v", tt.expected, result)
			}
		})
	}
}

func TestBatchContactJoinComma(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "整数切片",
			input:    []int{13800138001, 13800138002, 13800138003},
			expected: `13800138001,13800138002,13800138003`,
		},
		{
			name:     "字符串切片",
			input:    []string{"a", "b", "c"},
			expected: `a,b,c`,
		},
		{
			name:     "接口切片",
			input:    []interface{}{1, "b", 3.14},
			expected: `1,b,3.14`,
		},
		{
			name:     "不支持的类型",
			input:    123,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BatchContactsJoinComma(tt.input)
			if result != tt.expected {
				t.Errorf("期望得到 %v，实际得到 %v", tt.expected, result)
			}
		})
	}
}

func TestMappingAndJoin(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		prefix   string
		suffix   string
		join     string
		expected string
	}{
		{
			name:     "整数切片带前后缀",
			input:    []int{1, 2, 3},
			prefix:   "num_",
			suffix:   "_end",
			join:     ",",
			expected: "num_1_end,num_2_end,num_3_end",
		},
		{
			name:     "字符串切片带引号",
			input:    []string{"a", "b", "c"},
			prefix:   "@",
			suffix:   "",
			join:     " ",
			expected: `@a @b @c`,
		},
		{
			name:     "接口切片带括号",
			input:    []interface{}{1, "b", 3.14},
			prefix:   "(",
			suffix:   ")",
			join:     "|",
			expected: "(1)|(b)|(3.14)",
		},
		{
			name:     "空前后缀",
			input:    []int{1, 2, 3},
			prefix:   "",
			suffix:   "",
			join:     "-",
			expected: "1-2-3",
		},
		{
			name:     "不支持的类型",
			input:    123,
			prefix:   "test_",
			suffix:   "_test",
			join:     ",",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MappingAndJoin(tt.input, tt.prefix, tt.suffix, tt.join)
			if result != tt.expected {
				t.Errorf("期望得到 %v，实际得到 %v", tt.expected, result)
			}
		})
	}
}
