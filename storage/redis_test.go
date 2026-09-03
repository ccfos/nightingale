package storage

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestMiniRedisMGet(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	err = rdb.Ping(context.Background()).Err()
	if err != nil {
		t.Fatalf("failed to ping miniredis: %v", err)
	}

	mp := make(map[string]interface{})
	mp["key1"] = "value1"
	mp["key2"] = "value2"
	mp["key3"] = "value3"

	err = MSet(context.Background(), rdb, mp, 0)
	if err != nil {
		t.Fatalf("failed to set miniredis value: %v", err)
	}

	ctx := context.Background()
	keys := []string{"key1", "key2", "key3", "key4"}
	vals := MGet(ctx, rdb, keys)

	expected := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3")}
	assert.Equal(t, expected, vals)
}
