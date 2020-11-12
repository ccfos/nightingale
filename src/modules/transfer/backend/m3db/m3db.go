package m3db

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/m3db/m3/src/dbnode/client"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/m3ninx/idx"
	"github.com/m3db/m3/src/query/block"
	"github.com/m3db/m3/src/query/storage/m3/consolidators"
	"github.com/m3db/m3/src/x/ident"
	xtime "github.com/m3db/m3/src/x/time"
	"github.com/toolkits/pkg/logger"
)

const (
	NID_NAME      = "__nid__"
	ENDPOINT_NAME = "__endpoint__"
	METRIC_NAME   = "__name__"
	SERIES_LIMIT  = 1000
	DOCS_LIMIT    = 100
)

type M3dbSection struct {
	Name        string               `yaml:"name"`
	Enabled     bool                 `yaml:"enabled"`
	Namespace   string               `yaml:"namespace"`
	DaysLimit   int                  `yaml:"daysLimit"`
	SeriesLimit int                  `yaml:"seriesLimit"`
	DocsLimit   int                  `yaml:"docsLimit"`
	MinStep     int                  `yaml:"minStep"`
	Config      client.Configuration `yaml:",inline"`
}

type Client struct {
	sync.RWMutex

	client client.Client
	active client.Session
	opts   client.Options

	namespace string
	config    *M3dbSection

	namespaceID ident.ID
}

func NewClient(cfg M3dbSection) (*Client, error) {
	client, err := cfg.Config.NewClient(client.ConfigurationParameters{})
	if err != nil {
		return nil, fmt.Errorf("unable to get new M3DB client: %v", err)
	}

	if cfg.MinStep == 0 {
		cfg.MinStep = 1
	}

	ret := &Client{
		namespace:   cfg.Namespace,
		config:      &cfg,
		client:      client,
		namespaceID: ident.StringID(cfg.Namespace),
	}

	if _, err := ret.session(); err != nil {
		return nil, fmt.Errorf("unable to get new M3DB session: %v", err)
	}

	return ret, nil
}

// Push2Queue: push Metrics with values into m3.dbnode
func (p *Client) Push2Queue(items []*dataobj.MetricValue) {
	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return
	}

	errCnt := 0
	for _, item := range items {
		if err := session.WriteTagged(
			p.namespaceID,
			mvID(item),
			ident.NewTagsIterator(mvTags(item)),
			time.Unix(item.Timestamp, 0),
			item.Value,
			xtime.Second,
			nil,
		); err != nil {
			logger.Errorf("unable to writeTagged: %s", err)
			errCnt++
		}
	}
	stats.Counter.Set("m3db.queue.err", errCnt)
}

// QueryData: || (|| endpoints...) (&& tags...)
func (p *Client) QueryData(inputs []dataobj.QueryData) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data, inputs: %+v", inputs)

	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	if len(inputs) == 0 {
		return nil
	}

	query, opts := p.config.queryDataOptions(inputs)
	ret, err := fetchTagged(session, p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable to query data: ", err)
		return nil
	}

	return ret
}

// QueryDataForUi: && (metric) (|| endpoints...) (&& tags...)
func (p *Client) QueryDataForUI(input dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
	logger.Debugf("query data for ui, input: %+v", input)

	if err := p.config.validateQueryDataForUI(&input); err != nil {
		logger.Errorf("input is invalid %s", err)
		return nil
	}

	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	query, opts := p.config.queryDataUIOptions(input)

	ret, err := fetchTagged(session, p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable to query data for ui: %s", err)
		return nil
	}

	return aggregateResp(ret, input)
}

// QueryMetrics: || (&& (endpoint)) (counter)...
// return all the values that tag == __name__
func (p *Client) QueryMetrics(input dataobj.EndpointsRecv) *dataobj.MetricResp {
	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	query, opts := p.config.queryMetricsOptions(input)

	tags, err := completeTags(session, p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable completeTags: ", err)
		return nil
	}
	return tagsMr(tags)
}

// QueryTagPairs: && (|| endpoints...) (|| metrics...)
// return all the tags that matches
func (p *Client) QueryTagPairs(input dataobj.EndpointMetricRecv) []dataobj.IndexTagkvResp {
	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	query, opts := p.config.queryTagPairsOptions(input)

	tags, err := completeTags(session, p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable completeTags: ", err)
		return nil
	}

	return []dataobj.IndexTagkvResp{*tagsIndexTagkvResp(tags)}
}

// QueryIndexByClude:  || (&& (|| endpoints...) (metric) (|| include...) (&& exclude..))
// return all the tags that matches
func (p *Client) QueryIndexByClude(inputs []dataobj.CludeRecv) (ret []dataobj.XcludeResp) {
	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	for _, input := range inputs {
		ret = append(ret, p.queryIndexByClude(session, input)...)
	}

	return
}

func (p *Client) queryIndexByClude(session client.Session, input dataobj.CludeRecv) []dataobj.XcludeResp {
	query, opts := p.config.queryIndexByCludeOptions(input)

	iter, _, err := session.FetchTaggedIDs(p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable FetchTaggedIDs: ", err)
		return nil
	}

	// group by endpoint-metric
	respMap := make(map[string]dataobj.XcludeResp)
	for iter.Next() {
		_, _, tagIter := iter.Current()

		resp := xcludeResp(tagIter)
		key := fmt.Sprintf("%s-%s", resp.Endpoint, resp.Metric)
		if v, ok := respMap[key]; ok {
			if len(resp.Tags) > 0 {
				v.Tags = append(v.Tags, resp.Tags[0])
			}
		} else {
			respMap[key] = resp
		}
	}

	if err := iter.Err(); err != nil {
		logger.Errorf("FetchTaggedIDs iter:", err)
		return nil
	}

	resp := make([]dataobj.XcludeResp, 0, len(respMap))
	for _, v := range respMap {
		resp = append(resp, v)
	}

	return resp
}

// QueryIndexByFullTags: && (|| endpoints...) (metric) (&& Tagkv...)
// return all the tags that matches
func (p *Client) QueryIndexByFullTags(inputs []dataobj.IndexByFullTagsRecv) []dataobj.IndexByFullTagsResp {
	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	ret := make([]dataobj.IndexByFullTagsResp, len(inputs))
	for i, input := range inputs {
		ret[i] = p.queryIndexByFullTags(session, input)
	}

	return ret
}

func (p *Client) queryIndexByFullTags(session client.Session, input dataobj.IndexByFullTagsRecv) (ret dataobj.IndexByFullTagsResp) {
	log.Printf("entering queryIndexByFullTags")

	ret = dataobj.IndexByFullTagsResp{
		Metric: input.Metric,
		Tags:   []string{},
		Step:   10,
		DsType: "GAUGE",
	}

	query, opts := p.config.queryIndexByFullTagsOptions(input)
	if query.Query.Equal(idx.NewAllQuery()) {
		ret.Endpoints = input.Endpoints
		log.Printf("all query")
		return
	}

	iter, _, err := session.FetchTaggedIDs(p.namespaceID, query, opts)
	if err != nil {
		logger.Errorf("unable FetchTaggedIDs: ", err)
		return
	}

	ret.Endpoints = input.Endpoints
	for iter.Next() {
		log.Printf("iter.next() ")
		_, _, tagIter := iter.Current()
		resp := xcludeResp(tagIter)
		if len(resp.Tags) > 0 {
			ret.Tags = append(ret.Tags, resp.Tags[0])
		}
	}
	if err := iter.Err(); err != nil {
		logger.Errorf("FetchTaggedIDs iter:", err)
	}

	return ret

}

// GetInstance: && (metric) (endpoint) (&& tags...)
// return: backend list which store the series
func (p *Client) GetInstance(metric, endpoint string, tags map[string]string) []string {
	session, err := p.session()
	if err != nil {
		logger.Errorf("unable to get m3db session: %s", err)
		return nil
	}

	adminSession, ok := session.(client.AdminSession)
	if !ok {
		logger.Errorf("unable to get an admin session")
		return nil
	}

	tm, err := adminSession.TopologyMap()
	if err != nil {
		logger.Errorf("unable to get topologyMap with admin seesion")
		return nil
	}

	hosts := []string{}
	for _, host := range tm.Hosts() {
		hosts = append(hosts, host.Address())
	}

	return hosts
}

func (s *Client) session() (client.Session, error) {
	s.RLock()
	session := s.active
	s.RUnlock()
	if session != nil {
		return session, nil
	}

	s.Lock()
	if s.active != nil {
		session := s.active
		s.Unlock()
		return session, nil
	}
	session, err := s.client.DefaultSession()
	if err != nil {
		s.Unlock()
		return nil, err
	}
	s.active = session
	s.Unlock()

	return session, nil
}

func (s *Client) Close() error {
	var err error
	s.Lock()
	if s.active != nil {
		err = s.active.Close()
	}
	s.Unlock()
	return err
}

func fetchTagged(
	session client.Session,
	namespace ident.ID,
	q index.Query,
	opts index.QueryOptions,
) ([]*dataobj.TsdbQueryResponse, error) {
	seriesIters, _, err := session.FetchTagged(namespace, q, opts)
	if err != nil {
		return nil, err
	}

	ret := []*dataobj.TsdbQueryResponse{}
	for _, seriesIter := range seriesIters.Iters() {
		v, err := seriesIterWalk(seriesIter)
		if err != nil {
			return nil, err
		}
		ret = append(ret, v)
	}

	return ret, nil
}

func completeTags(
	session client.Session,
	namespace ident.ID,
	query index.Query,
	opts index.AggregationOptions,
) (*consolidators.CompleteTagsResult, error) {
	aggTagIter, metadata, err := session.Aggregate(namespace, query, opts)
	if err != nil {
		return nil, err
	}
	completedTags := make([]consolidators.CompletedTag, 0, aggTagIter.Remaining())
	for aggTagIter.Next() {
		name, values := aggTagIter.Current()
		tagValues := make([][]byte, 0, values.Remaining())
		for values.Next() {
			tagValues = append(tagValues, values.Current().Bytes())
		}
		if err := values.Err(); err != nil {
			return nil, err
		}
		completedTags = append(completedTags, consolidators.CompletedTag{
			Name:   name.Bytes(),
			Values: tagValues,
		})
	}
	if err := aggTagIter.Err(); err != nil {
		return nil, err
	}
	blockMeta := block.NewResultMetadata()
	blockMeta.Exhaustive = metadata.Exhaustive
	return &consolidators.CompleteTagsResult{
		CompleteNameOnly: opts.Type == index.AggregateTagNames,
		CompletedTags:    completedTags,
		Metadata:         blockMeta,
	}, nil
}

func seriesIterWalk(iter encoding.SeriesIterator) (out *dataobj.TsdbQueryResponse, err error) {
	values := []*dataobj.RRDData{}
	for iter.Next() {
		dp, _, _ := iter.Current()
		logger.Printf("%s: %v", dp.Timestamp.String(), dp.Value)
		values = append(values, dataobj.NewRRDData(dp.Timestamp.Unix(), dp.Value))
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	tagsIter := iter.Tags()
	tags := map[string]string{}
	for tagsIter.Next() {
		tag := tagsIter.Current()
		tags[tag.Name.String()] = tag.Value.String()
	}
	metric := tags[METRIC_NAME]
	endpoint := tags[ENDPOINT_NAME]
	counter, err := dataobj.GetCounter(metric, "", tags)

	return &dataobj.TsdbQueryResponse{
		Start:    iter.Start().Unix(),
		End:      iter.End().Unix(),
		Endpoint: endpoint,
		Counter:  counter,
		Values:   values,
	}, nil
}

func (cfg M3dbSection) validateQueryDataForUI(in *dataobj.QueryDataForUI) (err error) {
	if in.AggrFunc != "" &&
		in.AggrFunc != "sum" &&
		in.AggrFunc != "avg" &&
		in.AggrFunc != "max" &&
		in.AggrFunc != "min" {
		return fmt.Errorf("%s is invalid aggrfunc", in.AggrFunc)
	}

	if err := cfg.validateTime(in.Start, in.End, &in.Step); err != nil {
		return err
	}

	return nil
}

func (cfg M3dbSection) validateTime(start, end int64, step *int) error {
	if end <= start {
		return fmt.Errorf("query time range is invalid end %d <= start %d", end, start)
	}

	if cfg.DaysLimit > 0 {
		if days := int((end - start) / 86400); days > cfg.DaysLimit {
			return fmt.Errorf("query time reange in invalid, daysLimit(%d/%d)", days, cfg.DaysLimit)
		}
	}

	if *step == 0 {
		*step = int((end - start) / 720)
	}

	if *step > cfg.MinStep {
		*step = cfg.MinStep
	}
	return nil
}
