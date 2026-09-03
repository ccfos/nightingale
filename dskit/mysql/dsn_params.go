package mysql

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// NormalizeDSNExtraParams turns a user-supplied MySQL DSN query fragment into a
// fragment go-sql-driver/mysql can parse, so that unknown keys are issued as
// `SET key = value` right after the connection is established.
//
// JDBC packs several assignments into one sessionVariables=k1=v1,k2=v2 pair,
// which the Go driver has no equivalent for; that one key is expanded into
// separate k1=%27v1%27&k2=%27v2%27 pairs. Every other key is passed through as
// written, so callers use plain go-sql-driver syntax for the rest.
func NormalizeDSNExtraParams(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "?")
	raw = strings.TrimPrefix(raw, "&")
	if raw == "" {
		return "", nil
	}

	var parts []string
	for _, segment := range strings.Split(raw, "&") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		key, value, found := strings.Cut(segment, "=")
		if !found {
			return "", fmt.Errorf("invalid segment %q: missing '='", segment)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return "", fmt.Errorf("invalid segment %q: empty key", segment)
		}

		if key == "sessionVariables" {
			expanded, err := expandSessionVariables(value)
			if err != nil {
				return "", err
			}
			parts = append(parts, expanded...)
			continue
		}
		parts = append(parts, segment)
	}

	return strings.Join(parts, "&"), nil
}

func expandSessionVariables(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("sessionVariables value is empty")
	}

	var parts []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		subKey, subValue, found := strings.Cut(item, "=")
		if !found {
			return nil, fmt.Errorf("invalid sessionVariables entry %q: missing '='", item)
		}
		subKey = strings.TrimSpace(subKey)
		if subKey == "" {
			return nil, fmt.Errorf("invalid sessionVariables entry %q: empty key", item)
		}

		encoded, err := encodeDSNParamValue(strings.TrimSpace(subValue))
		if err != nil {
			return nil, err
		}
		parts = append(parts, subKey+"="+encoded)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("sessionVariables value is empty")
	}
	return parts, nil
}

// encodeDSNParamValue quotes string values with %27 as the driver requires, and
// leaves numbers and booleans bare.
func encodeDSNParamValue(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("empty session variable value")
	}
	if len(value) >= 6 && strings.HasPrefix(value, "%27") && strings.HasSuffix(value, "%27") {
		if _, err := url.QueryUnescape(value); err != nil {
			return "", fmt.Errorf("invalid url-encoded session variable value %q: %w", value, err)
		}
		return value, nil
	}

	unquoted := value
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		unquoted = value[1 : len(value)-1]
	}

	if isUnquotedDSNValue(unquoted) {
		return unquoted, nil
	}

	return "%27" + url.QueryEscape(unquoted) + "%27", nil
}

func isUnquotedDSNValue(value string) bool {
	switch strings.ToLower(value) {
	case "true", "false":
		return true
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return true
	}
	return false
}
