package common

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	statusAPIError = 422

	apiPrefix = "/api/v1"

	EpAlerts          = apiPrefix + "/alerts"
	EpAlertManagers   = apiPrefix + "/alertmanagers"
	EpQuery           = apiPrefix + "/query"
	EpQueryRange      = apiPrefix + "/query_range"
	EpLabels          = apiPrefix + "/labels"
	EpLabelValues     = apiPrefix + "/label/:name/values"
	EpSeries          = apiPrefix + "/series"
	EpTargets         = apiPrefix + "/targets"
	EpTargetsMetadata = apiPrefix + "/targets/metadata"
	EpMetadata        = apiPrefix + "/metadata"
	EpRules           = apiPrefix + "/rules"
	EpSnapshot        = apiPrefix + "/admin/tsdb/snapshot"
	EpDeleteSeries    = apiPrefix + "/admin/tsdb/delete_series"
	EpCleanTombstones = apiPrefix + "/admin/tsdb/clean_tombstones"
	EpConfig          = apiPrefix + "/status/config"
	EpFlags           = apiPrefix + "/status/flags"

	// Possible values for ErrorType.
	ErrBadData     ErrorType = "bad_data"
	ErrTimeout     ErrorType = "timeout"
	ErrCanceled    ErrorType = "canceled"
	ErrExec        ErrorType = "execution"
	ErrBadResponse ErrorType = "bad_response"
	ErrServer      ErrorType = "server_error"
	ErrClient      ErrorType = "client_error"
)

// ErrorType models the different API error types.
type ErrorType string

type ApiResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType ErrorType       `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// Error is an error returned by the API.
type Error struct {
	Type   ErrorType
	Msg    string
	Detail string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

func ApiError(code int) bool {
	// These are the codes that Prometheus sends when it returns an error.
	return code == statusAPIError || code == http.StatusBadRequest
}

func ErrorTypeAndMsgFor(resp *http.Response) (ErrorType, string) {
	switch resp.StatusCode / 100 {
	case 4:
		return ErrClient, fmt.Sprintf("client error: %d", resp.StatusCode)
	case 5:
		return ErrServer, fmt.Sprintf("server error: %d", resp.StatusCode)
	}
	return ErrBadResponse, fmt.Sprintf("bad response code %d", resp.StatusCode)
}
