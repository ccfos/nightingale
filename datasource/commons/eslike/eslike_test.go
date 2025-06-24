package eslike

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/sqlbase"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/stretchr/testify/assert"
)

// TestConfig holds configuration for ES tests
type TestConfig struct {
	ESAddress string
	Username  string
	Password  string
}

var testConfig = TestConfig{
	ESAddress: "http://localhost:9200", // Default test ES address
	Username:  "elastic",               // Add your test ES username
	Password:  "*",                     // Add your test ES password
}

// setupTestESClient creates a real ES client for integration tests
func setupTestESClient(t *testing.T) *elasticsearch.Client {
	cfg := elasticsearch.Config{
		Addresses: []string{testConfig.ESAddress},
	}
	if testConfig.Username != "" && testConfig.Password != "" {
		cfg.Username = testConfig.Username
		cfg.Password = testConfig.Password
	}

	client, err := elasticsearch.NewClient(cfg)
	assert.NoError(t, err)
	return client
}

func TestQuerySQLData(t *testing.T) {
	client := setupTestESClient(t)
	if client == nil {
		t.Skip("Skipping test: ES client not available")
	}

	// Test query parameters
	query := &Query{
		Query:        "SELECT * FROM library",
		Timeout:      30,
		QueryType:    "SQL",
		CustomParams: make(map[string]interface{}),
		MaxQueryRows: 2,
	}

	// Execute query
	results, err := QuerySQLData(context.Background(), query, 30, "7.0", client)
	assert.NoError(t, err)
	assert.NotNil(t, results)

	// Print results
	fmt.Printf("\nQuerySQLData Results:\n")
	for i, result := range results {
		fmt.Printf("Result %d:\n", i+1)
		fmt.Printf("  Ref: %s\n", result.Ref)
		fmt.Printf("  Metric: %v\n", result.Metric)
		fmt.Printf("  Values: %v\n", result.Values)
		if result.Query != "" {
			fmt.Printf("  Query: %s\n", result.Query)
		}
	}
}

func TestQuerySQLLog(t *testing.T) {
	client := setupTestESClient(t)
	if client == nil {
		t.Skip("Skipping test: ES client not available")
	}

	// Test query parameters
	query := &Query{
		Query:        "SELECT * FROM library",
		Timeout:      30,
		QueryType:    "SQL",
		CustomParams: make(map[string]interface{}),
		MaxQueryRows: 2,
	}

	// Execute query
	results, total, err := QuerySQLLog(context.Background(), query, 30, "7.0", client)
	assert.NoError(t, err)
	assert.NotNil(t, results)
	assert.GreaterOrEqual(t, total, int64(0))

	// Print results
	fmt.Printf("\nQuerySQLLog Results:\n")
	fmt.Printf("Total: %d\n", total)
	for i, result := range results {
		fmt.Printf("Result %d:\n", i+1)
		// Pretty print the result map
		if resultMap, ok := result.(map[string]interface{}); ok {
			prettyJSON, err := json.MarshalIndent(resultMap, "", "  ")
			if err == nil {
				fmt.Printf("  %s\n", string(prettyJSON))
			} else {
				fmt.Printf("  %v\n", resultMap)
			}
		} else {
			fmt.Printf("  %v\n", result)
		}
	}
}

func TestParseTime(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "RFC3339Nano format",
			input:    "2024-01-01T00:00:00.000Z",
			expected: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "Unix timestamp",
			input:    "1704067200",
			expected: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			wantErr:  true,
		},
		{
			name:     "Invalid format",
			input:    "invalid-time",
			expected: time.Time{},
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := sqlbase.ParseTime(tc.input, time.RFC3339Nano)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected.UTC(), result.UTC())
			}
		})
	}
}
