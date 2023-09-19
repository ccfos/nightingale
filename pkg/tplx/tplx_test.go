package tplx

import (
	"reflect"
	"testing"
)

func TestExtractTemplateVariables(t *testing.T) {

	testCases := []struct {
		input    string
		expected []string
	}{
		{
			`Hello {{ .name }}`,
			[]string{"name"},
		},
		{
			`Hello {{.name}} and {{.Age}}`,
			[]string{"name", "Age"},
		},
		{
			`No variables`,
			[]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			actual := ExtractTemplateVariables(tc.input)
			equal := reflect.DeepEqual(tc.expected, actual)
			if equal != true {
				t.Errorf("ExtractTemplateVariables() got = %v, want %v", actual, tc.expected)
			}
		})
	}

}

func TestReplaceMacroVariables(t *testing.T) {

	type macroStruct struct {
		Username string
		Password string
	}

	tests := []struct {
		name         string
		templateText string
		macroValue   any
		want         string
		wantErr      bool
	}{
		{
			name:         "basic",
			templateText: "username: {{ .Username }}, password: {{   .Password}}",
			macroValue: macroStruct{
				Username: "myuser",
				Password: "secret123",
			},
			want: "username: myuser, password: secret123",
		},
		{
			name:         "missing variable",
			templateText: "username: {{.Username}}",
			macroValue:   macroStruct{},
			want:         "username: ",
		}, {
			name:         "map struct macro value variable",
			templateText: "username: {{.username}}",
			macroValue:   map[string]string{"username": "bobo"},
			want:         "username: bobo",
		}, {
			name:         "error template",
			templateText: "username: {{.username{}}}",
			macroValue:   map[string]string{"username": "bobo"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReplaceMacroVariables(tt.name, tt.templateText, tt.macroValue)

			if (err != nil) == tt.wantErr {
				t.Logf("ReplaceMacroVariables() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got.String() != tt.want {
				t.Errorf("ReplaceMacroVariables() = %v, want %v", got.String(), tt.want)
			}
		})
	}
}
