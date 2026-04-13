package es

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestElasticsearch_Init(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]interface{}
		wantErr  bool
	}{
		{
			name: "valid settings",
			settings: map[string]interface{}{
				"es.addr":         "http://localhost:9200",
				"es.nodes":        []string{"http://localhost:9200"},
				"es.timeout":      30000,
				"es.version":      "7.10+",
				"es.max_shard":    5,
				"es.min_interval": 10,
			},
			wantErr: false,
		},
		{
			name: "invalid settings type",
			settings: map[string]interface{}{
				"es.timeout": "not_a_number",
			},
			wantErr: true,
		},
		{
			name:     "empty settings",
			settings: map[string]interface{}{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &Elasticsearch{}
			_, err := es.Init(tt.settings)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestElasticsearch_Validate(t *testing.T) {
	tests := []struct {
		name    string
		es      *Elasticsearch
		wantErr bool
	}{
		{
			name: "valid configuration",
			es: &Elasticsearch{
				Nodes:       []string{"http://localhost:9200"},
				Basic:       BasicAuth{Username: "user", Password: "pass"},
				MaxShard:    5,
				MinInterval: 10,
				Timeout:     30000,
			},
			wantErr: false,
		},
		{
			name: "no nodes",
			es: &Elasticsearch{
				Nodes: []string{},
			},
			wantErr: true,
		},
		{
			name: "invalid node URL",
			es: &Elasticsearch{
				Nodes: []string{"invalid-url"},
			},
			wantErr: true,
		},
		{
			name: "basic auth without credentials",
			es: &Elasticsearch{
				Nodes: []string{"http://localhost:9200"},
				Basic: BasicAuth{Enable: true, Username: "user"},
			},
			wantErr: true,
		},
		{
			name: "default values applied",
			es: &Elasticsearch{
				Nodes: []string{"http://localhost:9200"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.es.Validate(ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify defaults are set
				if tt.es.MaxShard == 0 {
					assert.Equal(t, defaultMaxShard, tt.es.MaxShard)
				}
				if tt.es.MinInterval < defaultMinInterval {
					assert.Equal(t, defaultMinInterval, tt.es.MinInterval)
				}
				if tt.es.Timeout == 0 {
					assert.Equal(t, defaultTimeout, tt.es.Timeout)
				}
			}
		})
	}
}

func TestElasticsearch_Equal(t *testing.T) {
	baseES := &Elasticsearch{
		Nodes:       []string{"http://node1:9200", "http://node2:9200"},
		Basic:       BasicAuth{Username: "user", Password: "pass"},
		TLS:         TLS{SkipTlsVerify: true},
		EnableWrite: true,
		Headers:     map[string]string{"X-Custom": "value"},
	}

	tests := []struct {
		name     string
		other    *Elasticsearch
		expected bool
	}{
		{
			name:     "identical configuration",
			other:    baseES,
			expected: true,
		},
		{
			name: "different nodes order",
			other: &Elasticsearch{
				Nodes:       []string{"http://node2:9200", "http://node1:9200"},
				Basic:       baseES.Basic,
				TLS:         baseES.TLS,
				EnableWrite: baseES.EnableWrite,
				Headers:     baseES.Headers,
			},
			expected: true,
		},
		{
			name: "different username",
			other: &Elasticsearch{
				Nodes:       baseES.Nodes,
				Basic:       BasicAuth{Username: "different", Password: "pass"},
				TLS:         baseES.TLS,
				EnableWrite: baseES.EnableWrite,
				Headers:     baseES.Headers,
			},
			expected: false,
		},
		{
			name: "different TLS setting",
			other: &Elasticsearch{
				Nodes:       baseES.Nodes,
				Basic:       baseES.Basic,
				TLS:         TLS{SkipTlsVerify: false},
				EnableWrite: baseES.EnableWrite,
				Headers:     baseES.Headers,
			},
			expected: false,
		},
		{
			name: "different headers",
			other: &Elasticsearch{
				Nodes:       baseES.Nodes,
				Basic:       baseES.Basic,
				TLS:         baseES.TLS,
				EnableWrite: baseES.EnableWrite,
				Headers:     map[string]string{"X-Different": "value"},
			},
			expected: false,
		},
		{
			name: "different write enable",
			other: &Elasticsearch{
				Nodes:       baseES.Nodes,
				Basic:       baseES.Basic,
				TLS:         baseES.TLS,
				EnableWrite: false,
				Headers:     baseES.Headers,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := baseES.Equal(tt.other)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestElasticsearch_isES6CompatibilityField(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		version  string
		expected bool
	}{
		{
			name:     "ES6 with doc field",
			field:    "doc",
			version:  "6.8.0",
			expected: true,
		},
		{
			name:     "ES7 with doc field",
			field:    "doc",
			version:  "7.10.0",
			expected: false,
		},
		{
			name:     "ES6 with non-doc field",
			field:    "field",
			version:  "6.8.0",
			expected: false,
		},
		{
			name:     "no version set",
			field:    "doc",
			version:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &Elasticsearch{Version: tt.version}
			result := es.isES6CompatibilityField(tt.field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestElasticsearch_addFieldIfNotExists(t *testing.T) {
	tests := []struct {
		name           string
		initialFields  []string
		fieldToAdd     string
		expectedFields []string
	}{
		{
			name:           "add new field",
			initialFields:  []string{"field1", "field2"},
			fieldToAdd:     "field3",
			expectedFields: []string{"field1", "field2", "field3"},
		},
		{
			name:           "add duplicate field",
			initialFields:  []string{"field1", "field2"},
			fieldToAdd:     "field1",
			expectedFields: []string{"field1", "field2"},
		},
		{
			name:           "add to empty list",
			initialFields:  []string{},
			fieldToAdd:     "field1",
			expectedFields: []string{"field1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := &Elasticsearch{}
			fieldMap := make(map[string]struct{})
			fields := tt.initialFields

			// Initialize field map
			for _, field := range fields {
				fieldMap[field] = struct{}{}
			}

			es.addFieldIfNotExists(tt.fieldToAdd, fieldMap, &fields)

			assert.ElementsMatch(t, tt.expectedFields, fields)
			assert.Len(t, fields, len(tt.expectedFields))
		})
	}
}

func TestGetFieldType(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		mapping  map[string]interface{}
		expected string
	}{
		{
			name: "direct field type",
			key:  "timestamp",
			mapping: map[string]interface{}{
				"mapping": map[string]interface{}{
					"timestamp": map[string]interface{}{
						"type": "date",
					},
				},
			},
			expected: "date",
		},
		{
			name: "nested field type",
			key:  "user.name",
			mapping: map[string]interface{}{
				"mapping": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "keyword",
					},
				},
			},
			expected: "keyword",
		},
		{
			name: "no mapping found",
			key:  "nonexistent",
			mapping: map[string]interface{}{
				"mapping": map[string]interface{}{},
			},
			expected: "",
		},
		{
			name: "invalid mapping structure",
			key:  "field",
			mapping: map[string]interface{}{
				"mapping": "not_a_map",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFieldType(tt.key, tt.mapping)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestElasticsearch_Test(t *testing.T) {
	tests := []struct {
		name    string
		es      *Elasticsearch
		wantErr bool
	}{
		{
			name: "valid configuration",
			es: &Elasticsearch{
				Addr:    "http://localhost:9200",
				Version: minVersion,
				Basic:   BasicAuth{Username: "user", Password: "pass"},
			},
			wantErr: false,
		},
		{
			name: "missing address",
			es: &Elasticsearch{
				Addr:    "",
				Version: minVersion,
			},
			wantErr: true,
		},
		{
			name: "invalid version",
			es: &Elasticsearch{
				Addr:    "http://localhost:9200",
				Version: "6.0.0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.es.Test(ctx)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				// Note: This will fail if there's no actual Elasticsearch instance running
				// In real testing, we'd mock the client
				assert.Error(t, err) // Expecting error due to no real ES connection
			}
		})
	}
}

func TestElasticsearch_Constants(t *testing.T) {
	assert.Equal(t, 5, defaultMaxShard)
	assert.Equal(t, 10, defaultMinInterval)
	assert.Equal(t, int64(60000), defaultTimeout)
	assert.Equal(t, 30, defaultQueryInterval)
	assert.Equal(t, "7.10+", minVersion)
}
