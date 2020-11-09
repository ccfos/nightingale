package m3db

import (
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/m3db/m3/src/dbnode/client"
	"github.com/m3db/m3/src/dbnode/encoding"
	"github.com/m3db/m3/src/dbnode/storage/index"
	"github.com/m3db/m3/src/query/block"
	"github.com/m3db/m3/src/query/storage/m3/consolidators"
	"github.com/m3db/m3/src/x/ident"
	"github.com/toolkits/pkg/logger"
	"k8s.io/klog/v2"

	xtime "github.com/m3db/m3/src/x/time"
)

type Session struct {
	client.Session
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

func writeTagged(session client.Session, namespace ident.ID, item *dataobj.MetricValue) error {
	return session.WriteTagged(namespace,
		mvID(item),
		ident.NewTagsIterator(mvTags(item)),
		time.Unix(item.Timestamp, 0),
		item.Value,
		xtime.Second,
		nil,
	)
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
		klog.Infof("Aggregate err %s", err)
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
