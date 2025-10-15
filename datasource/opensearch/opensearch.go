package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/datasource/commons/eslike"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/mitchellh/mapstructure"
	"github.com/olivere/elastic/v7"
	oscliv2 "github.com/opensearch-project/opensearch-go/v2"
	osapiv2 "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

const (
	OpenSearchType = "opensearch"
)

type OpenSearch struct {
	Addr        string            `json:"os.addr" mapstructure:"os.addr"`
	Nodes       []string          `json:"os.nodes" mapstructure:"os.nodes"`
	Timeout     int64             `json:"os.timeout" mapstructure:"os.timeout"` // millis
	Basic       BasicAuth         `json:"os.basic" mapstructure:"os.basic"`
	TLS         TLS               `json:"os.tls" mapstructure:"os.tls"`
	Version     string            `json:"os.version" mapstructure:"os.version"`
	Headers     map[string]string `json:"os.headers" mapstructure:"os.headers"`
	MinInterval int               `json:"os.min_interval" mapstructure:"os.min_interval"` // seconds
	MaxShard    int               `json:"os.max_shard" mapstructure:"os.max_shard"`
	ClusterName string            `json:"os.cluster_name" mapstructure:"os.cluster_name"`
	Client      *oscliv2.Client   `json:"os.client" mapstructure:"os.client"`
}

type TLS struct {
	SkipTlsVerify bool `json:"os.tls.skip_tls_verify" mapstructure:"os.tls.skip_tls_verify"`
}

type BasicAuth struct {
	Enable   bool   `json:"os.auth.enable" mapstructure:"os.auth.enable"`
	Username string `json:"os.user" mapstructure:"os.user"`
	Password string `json:"os.password" mapstructure:"os.password"`
}

func init() {
	datasource.RegisterDatasource(OpenSearchType, new(OpenSearch))
}

func (os *OpenSearch) Init(settings map[string]interface{}) (datasource.Datasource, error) {
	newest := new(OpenSearch)
	err := mapstructure.Decode(settings, newest)
	return newest, err
}

func (os *OpenSearch) InitClient() error {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(os.Timeout) * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: time.Duration(os.Timeout) * time.Millisecond,
	}

	if len(os.Nodes) > 0 {
		os.Addr = os.Nodes[0]
	}

	if strings.Contains(os.Addr, "https") {
		tlsConfig := tlsx.ClientConfig{
			InsecureSkipVerify: os.TLS.SkipTlsVerify,
			UseTLS:             true,
		}
		cfg, err := tlsConfig.TLSConfig()
		if err != nil {
			return err
		}
		transport.TLSClientConfig = cfg
	}

	headers := http.Header{}
	for k, v := range os.Headers {
		headers[k] = []string{v}
	}

	options := oscliv2.Config{
		Addresses: os.Nodes,
		Transport: transport,
		Header:    headers,
	}

	// 只要有用户名就添加认证，不依赖 Enable 字段
	if os.Basic.Username != "" {
		options.Username = os.Basic.Username
		options.Password = os.Basic.Password
	}

	var err = error(nil)
	os.Client, err = oscliv2.NewClient(options)

	return err
}

func (os *OpenSearch) Equal(other datasource.Datasource) bool {
	sort.Strings(os.Nodes)
	sort.Strings(other.(*OpenSearch).Nodes)

	if strings.Join(os.Nodes, ",") != strings.Join(other.(*OpenSearch).Nodes, ",") {
		return false
	}

	if os.Basic.Username != other.(*OpenSearch).Basic.Username {
		return false
	}

	if os.Basic.Password != other.(*OpenSearch).Basic.Password {
		return false
	}

	if os.TLS.SkipTlsVerify != other.(*OpenSearch).TLS.SkipTlsVerify {
		return false
	}

	if os.Timeout != other.(*OpenSearch).Timeout {
		return false
	}

	if !reflect.DeepEqual(os.Headers, other.(*OpenSearch).Headers) {
		return false
	}

	return true
}

func (os *OpenSearch) Validate(ctx context.Context) (err error) {
	if len(os.Nodes) == 0 {
		return fmt.Errorf("need a valid addr")
	}

	for _, addr := range os.Nodes {
		_, err = url.Parse(addr)
		if err != nil {
			return fmt.Errorf("parse addr error: %v", err)
		}
	}

	// 如果提供了用户名，必须同时提供密码
	if len(os.Basic.Username) > 0 && len(os.Basic.Password) == 0 {
		return fmt.Errorf("password is required when username is provided")
	}

	if os.MaxShard == 0 {
		os.MaxShard = 5
	}

	if os.MinInterval < 10 {
		os.MinInterval = 10
	}

	if os.Timeout == 0 {
		os.Timeout = 6000
	}

	if !strings.HasPrefix(os.Version, "2") {
		return fmt.Errorf("version must be 2.0+")
	}

	return nil
}

func (os *OpenSearch) MakeLogQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return eslike.MakeLogQuery(ctx, query, eventTags, start, end)
}

func (os *OpenSearch) MakeTSQuery(ctx context.Context, query interface{}, eventTags []string, start, end int64) (interface{}, error) {
	return eslike.MakeTSQuery(ctx, query, eventTags, start, end)
}

func search(ctx context.Context, indices []string, source interface{}, timeout int, cli *oscliv2.Client) (*elastic.SearchResult, error) {
	var body *bytes.Buffer = nil
	if source != nil {
		body = new(bytes.Buffer)
		err := json.NewEncoder(body).Encode(source)
		if err != nil {
			return nil, err
		}
	}

	req := osapiv2.SearchRequest{
		Index: indices,
		Body:  body,
	}

	if timeout > 0 {
		req.Timeout = time.Second * time.Duration(timeout)
	}

	resp, err := req.Do(ctx, cli)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opensearch response not 2xx, resp is %v", resp)
	}

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result := new(elastic.SearchResult)
	err = json.Unmarshal(bs, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (os *OpenSearch) QueryData(ctx context.Context, queryParam interface{}) ([]models.DataResp, error) {

	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		return search(ctx, indices, source, timeout, os.Client)
	}

	return eslike.QueryData(ctx, queryParam, os.Timeout, os.Version, search)
}

func (os *OpenSearch) QueryIndices() ([]string, error) {

	cir := osapiv2.CatIndicesRequest{
		Format: "json",
	}

	rsp, err := cir.Do(context.Background(), os.Client)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()

	bs, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}

	resp := make([]struct {
		Index string `json:"index"`
	}, 0)

	err = json.Unmarshal(bs, &resp)
	if err != nil {
		return nil, err
	}

	var ret []string
	for _, k := range resp {
		ret = append(ret, k.Index)
	}

	return ret, nil
}

func (os *OpenSearch) QueryFields(indices []string) ([]string, error) {

	var fields []string
	mappingRequest := osapiv2.IndicesGetMappingRequest{
		Index: indices,
	}

	resp, err := mappingRequest.Do(context.Background(), os.Client)
	if err != nil {
		return fields, err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return fields, err
	}

	result := map[string]interface{}{}

	err = json.Unmarshal(bs, &result)
	if err != nil {
		return fields, err
	}

	idx := ""
	if len(indices) > 0 {
		idx = indices[0]
	}

	mappingIndex := ""
	indexReg, _ := regexp.Compile(idx)
	for key, value := range result {
		mappings, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		if len(mappings) == 0 {
			continue
		}
		if key == idx || strings.Contains(key, idx) ||
			(indexReg != nil && indexReg.MatchString(key)) {
			mappingIndex = key
			break
		}
	}

	if len(mappingIndex) == 0 {
		return fields, nil
	}

	fields = propertyMappingRange(result[mappingIndex], 1)

	sort.Strings(fields)
	return fields, nil
}

func propertyMappingRange(v interface{}, depth int) (fields []string) {
	mapping, ok := v.(map[string]interface{})
	if !ok {
		return
	}
	if len(mapping) == 0 {
		return
	}
	for key, value := range mapping {
		if reflect.TypeOf(value).Kind() == reflect.Map {
			valueMap := value.(map[string]interface{})
			if prop, found := valueMap["properties"]; found {
				subFields := propertyMappingRange(prop, depth+1)
				for i := range subFields {
					if depth == 1 {
						fields = append(fields, subFields[i])
					} else {
						fields = append(fields, key+"."+subFields[i])
					}
				}
			} else if typ, found := valueMap["type"]; found {
				if eslike.HitFilter(typ.(string)) {
					continue
				}
				fields = append(fields, key)
			}
		}
	}
	return
}

func (os *OpenSearch) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {

	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		return search(ctx, indices, source, timeout, os.Client)
	}

	return eslike.QueryLog(ctx, queryParam, os.Timeout, os.Version, 0, search)
}

func (os *OpenSearch) QueryFieldValue(indexs []string, field string, query string) ([]string, error) {
	var values []string
	source := elastic.NewSearchSource().
		Size(0)

	if query != "" {
		source = source.Query(elastic.NewBoolQuery().Must(elastic.NewQueryStringQuery(query)))
	}
	source = source.Aggregation("distinct", elastic.NewTermsAggregation().Field(field).Size(10000))

	result, err := search(context.Background(), indexs, source, 0, os.Client)
	if err != nil {
		return values, err
	}

	agg, found := result.Aggregations.Terms("distinct")
	if !found {
		return values, nil
	}

	for _, bucket := range agg.Buckets {
		values = append(values, bucket.Key.(string))
	}

	return values, nil
}

func (os *OpenSearch) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	return nil, nil
}
