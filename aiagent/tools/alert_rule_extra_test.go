package tools

import (
	"testing"
)

func TestParseDisabledFlag(t *testing.T) {
	cases := []struct {
		name    string
		in      interface{}
		want    int
		wantErr bool
	}{
		{"nil → 0", nil, 0, false},
		{"empty string → 0", "", 0, false},
		{"float 0", float64(0), 0, false},
		{"float 1", float64(1), 1, false},
		{"int 0", 0, 0, false},
		{"int 1", 1, 1, false},
		{"string '0'", "0", 0, false},
		{"string '1'", "1", 1, false},

		{"negative float", float64(-1), 0, true},   // 现在 -1 必须报错
		{"negative int", -1, 0, true},
		{"negative string", "-1", 0, true},
		{"out of range 2", float64(2), 0, true},
		{"non-integer float", 0.5, 0, true},
		{"garbage string", "abc", 0, true},
		{"wrong type", []int{0}, 0, true},
	}
	for _, c := range cases {
		got, err := parseDisabledFlag(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("[%s] err=%v wantErr=%v", c.name, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("[%s] got %d want %d", c.name, got, c.want)
		}
	}
}
