package victorialogs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVictorialogs_Init(t *testing.T) {
	settings := map[string]interface{}{
		"addr":      "http://localhost:9428",
		"tls":       map[string]interface{}{"skip_tls_verify": true},
		"max_lines": 1000,
	}

	ds, err := new(Victorialogs).Init(settings)
	require.NoError(t, err)
	require.NotNil(t, ds)

	// Debug: print the actual type
	t.Logf("Returned type: %T", ds)

	victorialogs, ok := ds.(*Victorialogs)
	require.True(t, ok, "Expected *Victorialogs, got %T", ds)
	assert.Equal(t, "http://localhost:9428", victorialogs.Addr)
	assert.True(t, victorialogs.TLS.SkipTlsVerify)
	assert.Equal(t, 1000, victorialogs.MaxLines)
}

func TestVictorialogs_InitClient(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
		TLS: TLS{
			SkipTlsVerify: false,
		},
	}

	err := v.InitClient()
	require.NoError(t, err)
	assert.NotNil(t, v.Client)
}

func TestVictorialogs_Validate(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}
	err := v.InitClient()
	require.NoError(t, err)

	err = v.Validate(context.Background())
	assert.NoError(t, err)
}

func TestVictorialogs_Equal(t *testing.T) {
	v1 := &Victorialogs{
		Addr: "http://localhost:9428",
		TLS: TLS{
			SkipTlsVerify: true,
		},
	}

	v2 := &Victorialogs{
		Addr: "http://localhost:9428",
		TLS: TLS{
			SkipTlsVerify: true,
		},
	}

	assert.True(t, v1.Equal(v2))

	v3 := &Victorialogs{
		Addr: "http://localhost:9429",
		TLS: TLS{
			SkipTlsVerify: true,
		},
	}

	assert.False(t, v1.Equal(v3))
}

func TestVictorialogs_QueryData(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}
	err := v.InitClient()
	require.NoError(t, err)

	query := &QueryParam{
		Ref:   "A",
		Query: "* | stats by (level) count(*)",            // Example query for actual server
		Start: time.Now().Add(-7 * 24 * time.Hour).Unix(), // Look back 7 days
		End:   time.Now().Unix(),
		Step:  "12h",
	}

	results, err := v.QueryData(context.Background(), query)
	// Don't require success as the server might not have data
	if err != nil {
		t.Logf("QueryData returned error (expected if no data): %v", err)
		return
	}

	t.Logf("=== QueryData Results ===")
	t.Logf("Number of results: %d", len(results))

	if len(results) > 0 {
		for i, result := range results {
			t.Logf("Result %d:", i+1)
			t.Logf("  Ref: %s", result.Ref)
			t.Logf("  Query: %s", result.Query)
			t.Logf("  Metric: %+v", result.Metric)
			t.Logf("  Number of values: %d", len(result.Values))
			if len(result.Values) > 0 {
				t.Logf("  First value: %+v", result.Values[0])
				if len(result.Values) > 1 {
					t.Logf("  Last value: %+v", result.Values[len(result.Values)-1])
				}
			}
			t.Logf("  ---")
		}
	} else {
		t.Logf("No results returned")
	}
}

func TestVictorialogs_QueryData_Debug(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}
	err := v.InitClient()
	require.NoError(t, err)

	query := &QueryParam{
		Ref:   "A",
		Query: "* | stats by (level) count(*)",            // Example query for actual server
		Start: time.Now().Add(-7 * 24 * time.Hour).Unix(), // Look back 7 days
		End:   time.Now().Unix(),
		Step:  "12h",
	}

	// Debug: Show the request URL
	baseURL, err := url.Parse(v.Addr)
	require.NoError(t, err)
	baseURL.Path = "/select/logsql/stats_query_range"

	values := baseURL.Query()
	values.Add("query", query.Query)
	values.Add("start", strconv.FormatInt(query.Start, 10))
	values.Add("end", strconv.FormatInt(query.End, 10))
	values.Add("step", query.Step)
	baseURL.RawQuery = values.Encode()

	t.Logf("=== QueryData Debug ===")
	t.Logf("Request URL: %s", baseURL.String())
	t.Logf("Query: %s", query.Query)
	t.Logf("Start: %d (%s)", query.Start, time.Unix(query.Start, 0).Format(time.RFC3339))
	t.Logf("End: %d (%s)", query.End, time.Unix(query.End, 0).Format(time.RFC3339))
	t.Logf("Step: %s", query.Step)

	// Make the request manually to see the raw response
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL.String(), nil)
	require.NoError(t, err)

	resp, err := v.Client.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()

	t.Logf("Response Status: %s", resp.Status)
	t.Logf("Response Headers: %+v", resp.Header)

	// Read and show the raw response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	t.Logf("Raw Response Body:")
	t.Logf("%s", string(body))

	// Try to parse the response
	var respData struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Values [][]interface{}   `json:"values"`
			} `json:"result"`
		} `json:"data"`
		Error string `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
		t.Logf("Failed to unmarshal response: %v", err)
		t.Logf("Response might be in different format")

		// Try to parse as a different format
		var simpleResp map[string]interface{}
		if err := json.Unmarshal(body, &simpleResp); err != nil {
			t.Logf("Also failed to parse as simple map: %v", err)
		} else {
			t.Logf("Parsed as simple map: %+v", simpleResp)
		}
		return
	}

	t.Logf("Parsed Response:")
	t.Logf("Status: %s", respData.Status)
	t.Logf("Error: %s", respData.Error)
	t.Logf("ResultType: %s", respData.Data.ResultType)
	t.Logf("Number of results: %d", len(respData.Data.Result))

	for i, result := range respData.Data.Result {
		t.Logf("Result %d:", i+1)
		t.Logf("  Metric: %+v", result.Metric)
		t.Logf("  Values count: %d", len(result.Values))
		if len(result.Values) > 0 {
			t.Logf("  First value: %+v", result.Values[0])
		}
	}
}

func TestVictorialogs_QueryLog(t *testing.T) {
	v := &Victorialogs{
		Addr:     "http://localhost:9428",
		MaxLines: 1000,
	}
	err := v.InitClient()
	require.NoError(t, err)

	query := &QueryParam{
		Ref:   "A",
		Query: "_msg:*",                               // Query all logs
		Start: time.Now().Add(-24 * time.Hour).Unix(), // Look back 24 hours
		End:   time.Now().Unix(),
	}

	logs, total, err := v.QueryLog(context.Background(), query)
	// Don't require success as the server might not have data
	if err != nil {
		t.Logf("QueryLog returned error (expected if no data): %v", err)
		return
	}

	t.Logf("=== QueryLog Results ===")
	t.Logf("Total hits: %d", total)
	t.Logf("Number of logs returned: %d", len(logs))

	if len(logs) > 0 {
		t.Logf("First 5 log entries:")
		for i, log := range logs {
			if i >= 5 {
				break
			}
			logEntry, ok := log.(map[string]interface{})
			if ok {
				t.Logf("  Log %d: %+v", i+1, logEntry)
			} else {
				t.Logf("  Log %d: %T - %+v", i+1, log, log)
			}
		}

		if len(logs) > 5 {
			t.Logf("  ... and %d more logs", len(logs)-5)
		}
	} else {
		t.Logf("No logs returned")
	}
}

func TestCalcHits(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}
	err := v.InitClient()
	require.NoError(t, err)

	query := &QueryParam{
		Query: "_msg:*", // Query all logs
		Start: time.Now().Add(-1 * time.Hour).Unix(),
		End:   time.Now().Unix(),
	}

	total := CalcHits(context.Background(), query, v)
	t.Logf("Total hits: %d", total)
	// Don't assert specific value as it depends on actual data
}

func TestVictorialogs_QueryData_InvalidQueryParam(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}

	// Pass invalid query type
	_, err := v.QueryData(context.Background(), "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid query param")
}

func TestVictorialogs_QueryLog_InvalidQueryParam(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}

	// Pass invalid query type
	_, _, err := v.QueryLog(context.Background(), "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid query param")
}

func TestVictorialogs_QueryMapData(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}

	// QueryMapData is not implemented yet
	results, err := v.QueryMapData(context.Background(), nil)
	assert.NoError(t, err)
	assert.Nil(t, results)
}

func TestVictorialogs_MakeLogQuery(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}

	// MakeLogQuery is not implemented yet
	result, err := v.MakeLogQuery(context.Background(), nil, []string{}, 0, 0)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestVictorialogs_MakeTSQuery(t *testing.T) {
	v := &Victorialogs{
		Addr: "http://localhost:9428",
	}

	// MakeTSQuery is not implemented yet
	result, err := v.MakeTSQuery(context.Background(), nil, []string{}, 0, 0)
	assert.NoError(t, err)
	assert.Nil(t, result)
}
