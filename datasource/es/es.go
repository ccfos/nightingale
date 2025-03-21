package es

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/datasource/commons/eslike"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/mitchellh/mapstructure"
	"github.com/olivere/elastic/v7"
	"github.com/toolkits/pkg/logger"
)

const (
	ESType = "elasticsearch"
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
	EnableWrite bool              `json:"es.enable_write" mapstructure:"es.enable_write"` // 允许写操作
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
	err := mapstructure.Decode(settings, newest)
	return newest, err
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
			return err
		}
		transport.TLSClientConfig = cfg
	}

	var err error
	options := []elastic.ClientOptionFunc{
		elastic.SetURL(e.Nodes...),
	}

	if e.Basic.Username != "" {
		options = append(options, elastic.SetBasicAuth(e.Basic.Username, e.Basic.Password))
	}

	headers := http.Header{}
	for k, v := range e.Headers {
		headers[k] = []string{v}
	}

	options = append(options, elastic.SetHeaders(headers))
	options = append(options, elastic.SetHttpClient(&http.Client{Transport: transport}))
	options = append(options, elastic.SetSniff(false))
	options = append(options, elastic.SetHealthcheck(false))

	e.Client, err = elastic.NewClient(options...)
	return err
}

func (e *Elasticsearch) Equal(other datasource.Datasource) bool {
	sort.Strings(e.Nodes)
	sort.Strings(other.(*Elasticsearch).Nodes)

	if strings.Join(e.Nodes, ",") != strings.Join(other.(*Elasticsearch).Nodes, ",") {
		return false
	}

	if e.Basic.Username != other.(*Elasticsearch).Basic.Username {
		return false
	}

	if e.Basic.Password != other.(*Elasticsearch).Basic.Password {
		return false
	}

	if e.TLS.SkipTlsVerify != other.(*Elasticsearch).TLS.SkipTlsVerify {
		return false
	}
	if e.EnableWrite != other.(*Elasticsearch).EnableWrite {
		return false
	}

	if !reflect.DeepEqual(e.Headers, other.(*Elasticsearch).Headers) {
		return false
	}

	return true
}

func (e *Elasticsearch) Validate(ctx context.Context) (err error) {
	if len(e.Nodes) == 0 {
		return fmt.Errorf("need a valid addr")
	}

	for _, addr := range e.Nodes {
		_, err = url.Parse(addr)
		if err != nil {
			return fmt.Errorf("parse addr error: %v", err)
		}
	}

	if e.Basic.Enable && (len(e.Basic.Username) == 0 || len(e.Basic.Password) == 0) {
		return fmt.Errorf("need a valid user, password")
	}

	if e.MaxShard == 0 {
		e.MaxShard = 5
	}

	if e.MinInterval < 10 {
		e.MinInterval = 10
	}

	if e.Timeout == 0 {
		e.Timeout = 60000
	}

	if !strings.HasPrefix(e.Version, "6") && !strings.HasPrefix(e.Version, "7") {
		return fmt.Errorf("version must be 6.0+ or 7.0+")
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

	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		return e.Client.Search().
			Index(indices...).
			IgnoreUnavailable(true).
			Source(source).
			Timeout(fmt.Sprintf("%ds", timeout)).
			MaxConcurrentShardRequests(maxShard).
			Do(ctx)
	}

	return eslike.QueryData(ctx, queryParam, e.Timeout, e.Version, search)
}

func (e *Elasticsearch) QueryIndices() ([]string, error) {
	result, err := e.Client.IndexNames()

	return result, err
}

func (e *Elasticsearch) QueryFields(indexs []string) ([]string, error) {
	var fields []string
	result, err := elastic.NewGetFieldMappingService(e.Client).Index(indexs...).IgnoreUnavailable(true).Do(context.Background())
	if err != nil {
		return fields, err
	}

	fieldMap := make(map[string]struct{})
	for _, indexMap := range result {
		if m, exists := indexMap.(map[string]interface{})["mappings"]; exists {
			for k, v := range m.(map[string]interface{}) {
				// 兼容 es6 版本
				if k == "doc" && strings.HasPrefix(e.Version, "6") {
					// if k == "doc" {
					for kk, vv := range v.(map[string]interface{}) {
						typ := getFieldType(kk, vv.(map[string]interface{}))
						if eslike.HitFilter(typ) {
							continue
						}

						if _, exsits := fieldMap[kk]; !exsits {
							fieldMap[kk] = struct{}{}
							fields = append(fields, kk)
						}
					}
				} else {
					// es7 版本
					typ := getFieldType(k, v.(map[string]interface{}))
					if eslike.HitFilter(typ) {
						continue
					}

					if _, exsits := fieldMap[k]; !exsits {
						fieldMap[k] = struct{}{}
						fields = append(fields, k)
					}
				}

			}
		}
	}

	sort.Strings(fields)
	return fields, nil
}

func (e *Elasticsearch) QueryLog(ctx context.Context, queryParam interface{}) ([]interface{}, int64, error) {

	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {
		// 应该是之前为了获取 fields 字段，做的这个兼容
		// fields, err := e.QueryFields(indices)
		// if err != nil {
		// 	logger.Warningf("query data error:%v", err)
		// 	return nil, err
		// }

		// if source != nil && strings.HasPrefix(e.Version, "7") {
		// 	source = source.(*elastic.SearchSource).DocvalueFields(fields...)
		// }

		return e.Client.Search().
			Index(indices...).
			IgnoreUnavailable(true).
			MaxConcurrentShardRequests(maxShard).
			Source(source).
			Timeout(fmt.Sprintf("%ds", timeout)).
			Do(ctx)
	}

	return eslike.QueryLog(ctx, queryParam, e.Timeout, e.Version, e.MaxShard, search)
}

func (e *Elasticsearch) QueryFieldValue(indexs []string, field string, query string) ([]string, error) {
	var values []string
	search := e.Client.Search().
		IgnoreUnavailable(true).
		Index(indexs...).
		Size(0)

	if query != "" {
		search = search.Query(elastic.NewBoolQuery().Must(elastic.NewQueryStringQuery(query)))
	}
	search = search.Aggregation("distinct", elastic.NewTermsAggregation().Field(field).Size(10000))

	result, err := search.Do(context.Background())
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

func (e *Elasticsearch) Test(ctx context.Context) (err error) {
	err = e.Validate(ctx)
	if err != nil {
		return err
	}

	if e.Addr == "" {
		return fmt.Errorf("addr is invalid")
	}

	if e.Version == "7.10+" {
		options := []elastic.ClientOptionFunc{
			elastic.SetURL(e.Addr),
		}

		if e.Basic.Enable {
			options = append(options, elastic.SetBasicAuth(e.Basic.Username, e.Basic.Password))
		}

		client, err := elastic.NewClient(options...)
		if err != nil {
			return fmt.Errorf("config is invalid:%v", err)
		}

		_, err = client.ElasticsearchVersion(e.Addr)
		if err != nil {
			return fmt.Errorf("config is invalid:%v", err)
		}
	} else {
		return fmt.Errorf("version must be 7.10+")
	}

	return nil
}

func getFieldType(key string, m map[string]interface{}) string {
	if innerMap, exists := m["mapping"]; exists {
		if innerM, exists := innerMap.(map[string]interface{})[key]; exists {
			if typ, exists := innerM.(map[string]interface{})["type"]; exists {
				return typ.(string)
			}
		} else {
			arr := strings.Split(key, ".")
			if innerM, exists := innerMap.(map[string]interface{})[arr[len(arr)-1]]; exists {
				if typ, exists := innerM.(map[string]interface{})["type"]; exists {
					return typ.(string)
				}
			}
		}
	}
	return ""
}

func (e *Elasticsearch) QueryMapData(ctx context.Context, query interface{}) ([]map[string]string, error) {
	search := func(ctx context.Context, indices []string, source interface{}, timeout int, maxShard int) (*elastic.SearchResult, error) {

		return e.Client.Search().
			Index(indices...).
			IgnoreUnavailable(true).
			Source(source).
			Timeout(fmt.Sprintf("%ds", timeout)).
			Do(ctx)
	}

	param := new(eslike.Query)
	if err := mapstructure.Decode(query, param); err != nil {
		return nil, err
	}
	// 扩大查询范围, 解决上一次查询消耗时间太多，导致本次查询时间范围起止时间，滞后问题
	param.Interval += 30

	res, _, err := eslike.QueryLog(ctx, param, e.Timeout, e.Version, e.MaxShard, search)
	if err != nil {
		return nil, err
	}

	var result []map[string]string
	for _, item := range res {
		logger.Debugf("query:%v item:%v", query, item)
		if itemMap, ok := item.(*elastic.SearchHit); ok {
			mItem := make(map[string]string)
			// 遍历 fields 字段的每个键值对
			sourceMap := make(map[string]interface{})
			err := json.Unmarshal(itemMap.Source, &sourceMap)
			if err != nil {
				logger.Warningf("unmarshal source%s error:%v", string(itemMap.Source), err)
				continue
			}

			for k, v := range sourceMap {
				mItem[k] = fmt.Sprintf("%v", v)
			}

			// 将处理好的 map 添加到 m 切片中
			result = append(result, mItem)

			// 只取第一条数据
			break
		}
	}

	return result, nil
}
