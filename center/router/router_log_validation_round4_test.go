package router

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestValidateLokiTimeRange_RequiresPositiveStartAndIncreasingEnd(t *testing.T) {
	cases := []struct {
		name  string
		start int64
		end   int64
		valid bool
	}{
		{name: "valid", start: 100, end: 101, valid: true},
		{name: "zero start", end: 100},
		{name: "negative start", start: -1, end: 100},
		{name: "equal", start: 100, end: 100},
		{name: "reversed", start: 100, end: 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) {
				validateLokiTimeRange(tc.start, tc.end)
				c.String(http.StatusOK, "valid")
			}

			w := identityRequest(t, rt, handler, nil, "0", nil)

			if tc.valid {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "valid", w.Body.String())
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Equal(t, "invalid time range", w.Body.String())
			}
		})
	}
}

func TestValidateVictoriaLogsTimeRange_RequiresPositiveStartAndIncreasingEnd(t *testing.T) {
	cases := []struct {
		name  string
		start int64
		end   int64
		valid bool
	}{
		{name: "valid", start: 100, end: 101, valid: true},
		{name: "zero start", end: 100},
		{name: "negative start", start: -1, end: 100},
		{name: "equal", start: 100, end: 100},
		{name: "reversed", start: 100, end: 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) {
				validateVictoriaLogsTimeRange(tc.start, tc.end)
				c.String(http.StatusOK, "valid")
			}

			w := identityRequest(t, rt, handler, nil, "0", nil)

			if tc.valid {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "valid", w.Body.String())
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Equal(t, "invalid time range", w.Body.String())
			}
		})
	}
}

func TestValidateLokiLimit_DefaultsAndCapsResultCount(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "negative", limit: -1, want: 75},
		{name: "zero", want: 75},
		{name: "positive", limit: 12, want: 12},
		{name: "maximum", limit: 1000, want: 1000},
		{name: "over maximum", limit: 1001, want: 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateLokiLimit(tc.limit, 75)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateLokiParsedLimit_DefaultsAndCapsFieldCount(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "negative", limit: -1, want: 200},
		{name: "zero", want: 200},
		{name: "positive", limit: 12, want: 12},
		{name: "maximum", limit: 500, want: 500},
		{name: "over maximum", limit: 501, want: 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateLokiParsedLimit(tc.limit)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateVictoriaLogsLimit_DefaultsAndCapsResultCount(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "negative", limit: -1, want: 20},
		{name: "zero", want: 20},
		{name: "positive", limit: 12, want: 12},
		{name: "maximum", limit: 100, want: 100},
		{name: "over maximum", limit: 101, want: 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateVictoriaLogsLimit(tc.limit, 20)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateLokiLabel_RejectsEmptyOversizedAndControlCharacters(t *testing.T) {
	cases := []struct {
		name    string
		label   string
		message string
	}{
		{name: "valid", label: "service_name"},
		{name: "maximum length", label: strings.Repeat("a", 512)},
		{name: "blank", label: " \t", message: "label is required"},
		{name: "oversized", label: strings.Repeat("a", 513), message: "label is too long"},
		{name: "newline", label: "service\nname", message: "invalid label"},
		{name: "delete character", label: "service\x7f", message: "invalid label"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) {
				validateLokiLabel(tc.label)
				c.String(http.StatusOK, "valid")
			}

			w := identityRequest(t, rt, handler, nil, "0", nil)

			if tc.message == "" {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "valid", w.Body.String())
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Equal(t, tc.message, w.Body.String())
			}
		})
	}
}

func TestValidateVictoriaLogsField_RejectsEmptyOversizedAndControlCharacters(t *testing.T) {
	cases := []struct {
		name    string
		field   string
		message string
	}{
		{name: "valid", field: "kubernetes.namespace"},
		{name: "maximum length", field: strings.Repeat("a", 512)},
		{name: "blank", field: " \t", message: "field is required"},
		{name: "oversized", field: strings.Repeat("a", 513), message: "field is too long"},
		{name: "newline", field: "service\nname", message: "invalid field"},
		{name: "delete character", field: "service\x7f", message: "invalid field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) {
				validateVictoriaLogsField(tc.field)
				c.String(http.StatusOK, "valid")
			}

			w := identityRequest(t, rt, handler, nil, "0", nil)

			if tc.message == "" {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "valid", w.Body.String())
			} else {
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Equal(t, tc.message, w.Body.String())
			}
		})
	}
}

func TestNormalizeVictoriaLogsFieldScope_RestrictsSupportedScopes(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		want  string
	}{
		{name: "default", want: "field"},
		{name: "field", scope: "field", want: "field"},
		{name: "stream", scope: "stream_field", want: "stream_field"},
		{name: "unsupported", scope: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) { c.String(http.StatusOK, normalizeVictoriaLogsFieldScope(tc.scope)) }

			w := identityRequest(t, rt, handler, nil, "0", nil)

			if tc.want == "" {
				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Equal(t, "invalid scope", w.Body.String())
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, tc.want, w.Body.String())
			}
		})
	}
}

func TestVictoriaLogsBuilderSuggestions_ExcludeReservedStreamFields(t *testing.T) {
	cases := []struct {
		field   string
		blocked bool
	}{
		{field: "_time", blocked: true},
		{field: "_msg", blocked: true},
		{field: "_stream", blocked: true},
		{field: "_stream_id", blocked: true},
		{field: "_stream.namespace", blocked: true},
		{field: "namespace"},
		{field: "_streaming"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got := isVictoriaLogsBuilderSuggestionBlockedField(tc.field)

			require.Equal(t, tc.blocked, got)
		})
	}
}
