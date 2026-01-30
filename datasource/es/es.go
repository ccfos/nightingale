package es

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/datasource/commons/eslike"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/mitchellh/mapstructure"
	"github.com/olivere/elastic/v7"

	"github.com/ccfos/nightingale/v6/pkg/logx"
)

const (
	ESType = "elasticsearch"

	defaultMaxShard      = 5
	defaultMinInterval   = 10
	defaultTimeout       = 60000
	defaultQueryInterval = 30
	minVersion           = "7.10+"
)

type Elasticsearch struct {
	Addr        string            `json:"es.addr" mapstructure:"es.addr"`
	Nodes       []string          `json:"es.nodes" mapstructure:"es.nodes"`
	Timeout     int64             `json:"es.timeout" mapstructure:"es.timeout"` // millis
	Basic       BasicAuth         `json:"es.basic" mapstructure:"es.basic"`
	TLS         TLS               `json:"es.tls" mapstructure:"es.tls"`
	Version     string            `json:"es.version" mapstructure:"es.version"`
	Headers     map[string]string `json:"es.headers" mapstructure:"es.headers"`
	MinInterval int               `json:"es.min_interval" mapstructure:"es.min_interval"` // seconds
	MaxShard    int               `json:"es.max_shard" mapstructure:"es.max_shard"`
	ClusterName string            `json:"es.cluster_name" mapstructure:"es.cluster_name"`
	EnableWrite bool              `json:"es.enable_write" mapstructure:"es.enable_write"` // Enable write operations
	Client      *elastic.Client   `json:"es.client" mapstructure:"es.client"`
}

type TLS struct {
	SkipTlsVerify bool `json:"es.tls.skip_tls_verify" mapstructure:"es.tls.skip_tls_verify"`
}

type BasicAuth struct {
	Enable   bool   `json:"es.auth.enable" mapstructure:"es.auth.enable"`
	Username string `json:"es.user" mapstructure:"es.user"`
	Password string `json:"es.password" mapstructure:"es.password"`
}

func init() {
	datasource.RegisterDatasource(ESType, new(Elasticsearch))
}

func (e *Elasticsearch) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(Elasticsearch)
	if err := mapstructure.Decode(settings, newest); err != nil {
		return nil, fmt.Errorf("failed to decode elasticsearch settings: %w", err)
	}
	return newest, nil
}

func (e *Elasticsearch) InitClient() error {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(e.Timeout) * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: time.Duration(e.Timeout) * time.Millisecond,
	}

	if len(e.Nodes) > 0 {
		e.Addr = e.Nodes[0]
	}

	if strings.Contains(e.Addr, "https") {
		tlsConfig := tlsx.ClientConfig{
			InsecureSkipVerify: e.TLS.SkipTlsVerify,
			UseTLS:             true,
		}
		cfg, err := tlsConfig.TLSConfig()
		if err != nil {
			return fmt.Errorf("failed to create elasticsearch TLS config: %w", err)
		}
		transport.TLSClientConfig = cfg
	}

	options := []elastic.ClientOptionFunc{
		elastic.SetURL(e.Nodes...),
		elastic.SetSniff(false),
		elastic.SetHealthcheck(false),
	}

	if e.Basic.Username != "" {
		options = append(options, elastic.SetBasicAuth(e.Basic.Username, e.Basic.Password))
	}

	if len(e.Headers) > 0 {
		headers := http.Header{}
		for k, v := range e.Headers {
			headers[k] = []string{v}
		}
		options = append(options, elastic.SetHeaders(headers))
	}

	options = append(options, elastic.SetHttpClient(&http.Client{
		Transport: transport,
		Timeout:   time.Duration(e.Timeout) * time.Millisecond,
	}))

	var err error
	e.Client, err = elastic.NewClient(options...)
	if err != nil {
		return fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	return nil
}

func (e *Elasticsearch) Equal(other datasource.Datasource) bool {
	otherES, ok := other.(*Elasticsearch)
	if !ok {
		return false
	}

	sort.Strings(e.Nodes)
	sort.Strings(otherES.Nodes)

	if strings.Join(e.Nodes, ",") != strings.Join(otherES.Nodes, ",") {
		return false
	}

	if e.Basic.Username != otherES.Basic.Username {
		return false
	}

	if e.Basic.Password != otherES.Basic.Password {
		return false
	}

	if e.TLS.SkipTlsVerify != otherES.TLS.SkipTlsVerify {
		return false
	}

	if e.EnableWrite != otherES.EnableWrite {
		return false
	}

	if len(e.Headers) != len(otherES.Headers) {
		return false
	}

	for k, v := range e.Headers {
		if otherES.Headers[k] != v {
			return false
		}
	}

	return true
}

func (e *Elasticsearch) Validate(ctx context.Context) error {
	if len(e.Nodes) == 0 {
		return fmt.Errorf("at least one node address is required")
	}

	for _, addr := range e.Nodes {
		if _, err := url.Parse(addr); err != nil {
			return fmt.Errorf("invalid node address %s: %w", addr, err)
		}
	}

	if e.Basic.Enable && (e.Basic.Username == "" || e.Basic.Password == "") {
		return fmt.Errorf("basic auth requires both username and password")
	}

	// Set defaults
	if e.MaxShard == 0 {
		e.MaxShard = defaultMaxShard
	}

	if e.MinInterval < defaultMinInterval {
		e.MinInterval = defaultMinInterval
	}

	if e.Timeout == 0 {
		e.Timeout = defaultTimeout
	}

	return nil
}

func (e *Elasticsearch) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return eslike.MakeLogQuery(ctx, query, eventTags, start, end)
}

func (e *Elasticsearch) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return eslike.MakeTSQuery(ctx, query, eventTags, start, end)
}

func (e *Elasticsearch) QueryData(ctx context.Context, queryParam interface{}) ([]models.DataResp, error) {
	searchFunc := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		return e.Client.Search().
			Index(indices...).
			IgnoreUnavailable(true).
			Source(source).
			Timeout(fmt.Sprintf("%ds", timeout)).
			MaxConcurrentShardRequests(maxShard).
			Do(ctx)
	}
	return eslike.QueryData(ctx, queryParam, e.Timeout, e.Version, searchFunc)
}

func (e *Elasticsearch) QueryIndices() ([]string, error) {
	indices, err := e.Client.IndexNames()
	if err != nil {
		return nil, fmt.Errorf("failed to get index names: %w", err)
	}
	return indices, nil
}

func (e *Elasticsearch) QueryFields(indexes []string) ([]string, error) {
	result, err := elastic.NewGetFieldMappingService(e.Client).
		Index(indexes...).
		IgnoreUnavailable(true).
		Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get field mappings: %w", err)
	}

	fieldMap := make(map[string]struct{})
	var fields []string

	for _, indexMap := range result {
		mappings, exists := indexMap.(map[string]interface{})["mappings"]
		if !exists {
			continue
		}

		mappingMap := mappings.(map[string]interface{})
		for fieldName, fieldData := range mappingMap {
			if e.isES6CompatibilityField(fieldName) {
				e.processES6Fields(fieldData, fieldMap, &fields)
			} else {
				e.processES7Field(fieldName, fieldData, fieldMap, &fields)
			}
		}
	}

	sort.Strings(fields)
	return fields, nil
}

func (e *Elasticsearch) isES6CompatibilityField(fieldName string) bool {
	return fieldName == "doc" && strings.HasPrefix(e.Version, "6")
}

func (e *Elasticsearch) processES6Fields(fieldData interface{}, fieldMap map[string]struct{}, fields *[]string) {
	fieldMapData, ok := fieldData.(map[string]interface{})
	if !ok {
		return
	}

	for fieldName, fieldProps := range fieldMapData {
		fieldType := getFieldType(fieldName, fieldProps.(map[string]interface{}))
		if eslike.HitFilter(fieldType) {
			continue
		}
		e.addFieldIfNotExists(fieldName, fieldMap, fields)
	}
}

func (e *Elasticsearch) processES7Field(fieldName string, fieldData interface{}, fieldMap map[string]struct{}, fields *[]string) {
	fieldType := getFieldType(fieldName, fieldData.(map[string]interface{}))
	if eslike.HitFilter(fieldType) {
		return
	}
	e.addFieldIfNotExists(fieldName, fieldMap, fields)
}

func (e *Elasticsearch) addFieldIfNotExists(fieldName string, fieldMap map[string]struct{}, fields *[]string) {
	if _, exists := fieldMap[fieldName]; !exists {
		fieldMap[fieldName] = struct{}{}
		*fields = append(*fields, fieldName)
	}
}

func (e *Elasticsearch) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {
	searchFunc := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		return e.Client.Search().
			Index(indices...).
			IgnoreUnavailable(true).
			MaxConcurrentShardRequests(maxShard).
			Source(source).
			Timeout(fmt.Sprintf("%ds", timeout)).
			Do(ctx)
	}

	return eslike.QueryLog(ctx, queryParam, e.Timeout, e.Version, e.MaxShard, searchFunc)
}

func (e *Elasticsearch) QueryFieldValue(indexes []string, field string, query string) ([]string, error) {
	searchService := e.Client.Search().
		IgnoreUnavailable(true).
		Index(indexes...).
		Size(0)

	if query != "" {
		searchService = searchService.Query(elastic.NewBoolQuery().Must(elastic.NewQueryStringQuery(query)))
	}

	searchService = searchService.Aggregation("distinct",
		elastic.NewTermsAggregation().Field(field).Size(10000))

	result, err := searchService.Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to query field values: %w", err)
	}

	agg, found := result.Aggregations.Terms("distinct")
	if !found {
		return []string{}, nil
	}

	values := make([]string, 0, len(agg.Buckets))
	for _, bucket := range agg.Buckets {
		if key, ok := bucket.Key.(string); ok {
			values = append(values, key)
		}
	}

	return values, nil
}

func (e *Elasticsearch) Test(ctx context.Context) error {
	if err := e.Validate(ctx); err != nil {
		return err
	}

	if e.Addr == "" {
		return fmt.Errorf("address is required")
	}

	if e.Version != minVersion {
		return fmt.Errorf("version must be %s", minVersion)
	}

	options := []elastic.ClientOptionFunc{
		elastic.SetURL(e.Addr),
	}

	if e.Basic.Enable {
		options = append(options, elastic.SetBasicAuth(e.Basic.Username, e.Basic.Password))
	}

	client, err := elastic.NewClient(options...)
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	if _, err := client.ElasticsearchVersion(e.Addr); err != nil {
		return fmt.Errorf("failed to connect to Elasticsearch: %w", err)
	}

	return nil
}

func getFieldType(key string, m map[string]interface{}) string {
	innerMap, exists := m["mapping"]
	if !exists {
		return ""
	}

	mapping, ok := innerMap.(map[string]interface{})
	if !ok {
		return ""
	}

	// Try direct key match
	if fieldData, exists := mapping[key]; exists {
		if fieldMap, ok := fieldData.(map[string]interface{}); ok {
			if typ, exists := fieldMap["type"]; exists {
				return typ.(string)
			}
		}
	}

	// Try nested field match (e.g., "field.subfield")
	parts := strings.Split(key, ".")
	if len(parts) > 1 {
		if fieldData, exists := mapping[parts[len(parts)-1]]; exists {
			if fieldMap, ok := fieldData.(map[string]interface{}); ok {
				if typ, exists := fieldMap["type"]; exists {
					return typ.(string)
				}
			}
		}
	}

	return ""
}

func (e *Elasticsearch) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	searchFunc := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		return e.Client.Search().
			Index(indices...).
			IgnoreUnavailable(true).
			Source(source).
			Timeout(fmt.Sprintf("%ds", timeout)).
			Do(ctx)
	}

	param := new(eslike.Query)
	if err := mapstructure.Decode(query, param); err != nil {
		return nil, fmt.Errorf("failed to decode query parameters: %w", err)
	}

	// Extend query interval to handle timing issues
	param.Interval += defaultQueryInterval

	results, _, err := eslike.QueryLog(ctx, param, e.Timeout, e.Version, e.MaxShard, searchFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to query log data: %w", err)
	}

	var mappedResults []map[string]string
	for _, item := range results {
		logx.Debugf(ctx, "query:%v item:%v", query, item)

		searchHit, ok := item.(*elastic.SearchHit)
		if !ok {
			continue
		}

		sourceMap := make(map[string]interface{})
		if err := json.Unmarshal(searchHit.Source, &sourceMap); err != nil {
			logx.Warningf(ctx, "failed to unmarshal source %s: %v", string(searchHit.Source), err)
			continue
		}

		mappedItem := make(map[string]string)
		for k, v := range sourceMap {
			mappedItem[k] = fmt.Sprintf("%v", v)
		}

		mappedResults = append(mappedResults, mappedItem)

		// Break if limit is not set (only get first result)
		if param.Limit <= 0 {
			break
		}
	}

	return mappedResults, nil
}
