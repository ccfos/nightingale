package mongodb

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type MongoDB struct {
	Shards []Shard `json:"mongodb.shards" mapstructure:"mongodb.shards"`
}

type Shard struct {
	URI           string            `json:"mongodb.uri" mapstructure:"mongodb.uri"`
	Hosts         []string          `json:"mongodb.hosts" mapstructure:"mongodb.hosts"`
	User          string            `json:"mongodb.user" mapstructure:"mongodb.user"`
	Password      string            `json:"mongodb.password" mapstructure:"mongodb.password"`
	AuthSource    string            `json:"mongodb.auth_source" mapstructure:"mongodb.auth_source"`
	ReplicaSet    string            `json:"mongodb.replica_set" mapstructure:"mongodb.replica_set"`
	Database      string            `json:"mongodb.database" mapstructure:"mongodb.database"`
	Timeout       int               `json:"mongodb.timeout" mapstructure:"mongodb.timeout"` // seconds
	MaxPoolSize   uint64            `json:"mongodb.max_pool_size" mapstructure:"mongodb.max_pool_size"`
	TLSEnable     bool              `json:"mongodb.tls_enable" mapstructure:"mongodb.tls_enable"`
	TLSSkipVerify bool              `json:"mongodb.tls_skip_verify" mapstructure:"mongodb.tls_skip_verify"`
	Params        map[string]string `json:"mongodb.params" mapstructure:"mongodb.params"`
}

func (m *MongoDB) firstShard() (*Shard, error) {
	if len(m.Shards) == 0 {
		return nil, errors.New("empty mongodb shards")
	}

	return &m.Shards[0], nil
}

func (m *MongoDB) NewClient(ctx context.Context) (*mongo.Client, error) {
	shard, err := m.firstShard()
	if err != nil {
		return nil, err
	}

	uri := shard.URI
	if uri == "" {
		if len(shard.Hosts) == 0 {
			return nil, errors.New("empty mongodb uri or hosts")
		}

		uri = "mongodb://" + strings.Join(shard.Hosts, ",")
	}

	clientKey := []string{uri, shard.User, shard.Password, shard.AuthSource, shard.ReplicaSet}
	cachedKey := strings.Join(clientKey, "|")
	if cli, ok := pool.PoolClient.Load(cachedKey); ok {
		return cli.(*mongo.Client), nil
	}

	clientOpts := options.Client().ApplyURI(uri)

	if shard.User != "" {
		clientOpts.Auth = &options.Credential{
			Username:   shard.User,
			Password:   shard.Password,
			AuthSource: shard.AuthSource,
		}
	}

	if shard.ReplicaSet != "" {
		clientOpts.SetReplicaSet(shard.ReplicaSet)
	}

	if shard.MaxPoolSize > 0 {
		clientOpts.SetMaxPoolSize(shard.MaxPoolSize)
	}

	if shard.TLSEnable {
		clientOpts.SetTLSConfig(&tls.Config{
			InsecureSkipVerify: shard.TLSSkipVerify,
		})
	}

	for k, v := range shard.Params {
		switch strings.ToLower(k) {
		case "appname":
			clientOpts.SetAppName(v)
		case "direct":
			if strings.EqualFold(v, "true") {
				clientOpts.SetDirect(true)
			}
		}
	}

	timeout := shard.Timeout
	if timeout <= 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	client, err := mongo.Connect(timeoutCtx, clientOpts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(timeoutCtx, readpref.Primary()); err != nil {
		return nil, err
	}

	pool.PoolClient.Store(cachedKey, client)
	return client, nil
}

func (m *MongoDB) Aggregate(ctx context.Context, database, collection string, pipeline mongo.Pipeline) ([]map[string]interface{}, error) {
	shard, err := m.firstShard()
	if err != nil {
		return nil, err
	}

	client, err := m.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	if database == "" {
		database = shard.Database
	}

	if database == "" {
		return nil, errors.New("empty mongodb database")
	}

	if collection == "" {
		return nil, errors.New("empty mongodb collection")
	}

	timeout := shard.Timeout
	if timeout <= 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cursor, err := client.Database(database).Collection(collection).Aggregate(timeoutCtx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(timeoutCtx)

	var rows []map[string]interface{}
	if err := cursor.All(timeoutCtx, &rows); err != nil {
		return nil, err
	}

	return normalizeDocuments(rows), nil
}

func (m *MongoDB) Find(ctx context.Context, database, collection string, filter interface{}, opts *options.FindOptions) ([]map[string]interface{}, error) {
	shard, err := m.firstShard()
	if err != nil {
		return nil, err
	}

	client, err := m.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	if database == "" {
		database = shard.Database
	}

	if database == "" {
		return nil, errors.New("empty mongodb database")
	}

	if collection == "" {
		return nil, errors.New("empty mongodb collection")
	}

	timeout := shard.Timeout
	if timeout <= 0 {
		timeout = 60
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cursor, err := client.Database(database).Collection(collection).Find(timeoutCtx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(timeoutCtx)

	var rows []map[string]interface{}
	if err := cursor.All(timeoutCtx, &rows); err != nil {
		return nil, err
	}

	return normalizeDocuments(rows), nil
}

func normalizeDocuments(items []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, len(items))
	for i := range items {
		out[i] = NormalizeValue(items[i]).(map[string]interface{})
	}
	return out
}

func NormalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		res := make(map[string]interface{}, len(val))
		for k, inner := range val {
			res[k] = NormalizeValue(inner)
		}
		return res
	case []interface{}:
		res := make([]interface{}, len(val))
		for i := range val {
			res[i] = NormalizeValue(val[i])
		}
		return res
	case float64:
		if math.Mod(val, 1) == 0 {
			if val <= math.MaxInt32 && val >= math.MinInt32 {
				return int32(val)
			}
			if val <= math.MaxInt64 && val >= math.MinInt64 {
				return int64(val)
			}
		}
		return val
	default:
		return val
	}
}

func ConvertPipeline(stages []map[string]interface{}) (mongo.Pipeline, error) {
	pipeline := make(mongo.Pipeline, 0, len(stages))
	for _, stage := range stages {
		normalized := NormalizeValue(stage)
		doc, ok := normalized.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid pipeline stage: %v", stage)
		}

		stageDoc := bson.D{}
		keys := make([]string, 0, len(doc))
		for k := range doc {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			stageDoc = append(stageDoc, bson.E{Key: k, Value: doc[k]})
		}

		pipeline = append(pipeline, stageDoc)
	}

	return pipeline, nil
}
