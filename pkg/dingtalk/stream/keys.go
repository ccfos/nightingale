package stream

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const redisKeyPrefix = "n9e:dingtalk:"

func leaderRedisKey(appKey string) string {
	h := sha256.Sum256([]byte(appKey))
	return fmt.Sprintf("%sstream:leader:%s", redisKeyPrefix, hex.EncodeToString(h[:8]))
}

func eventDedupeRedisKey(eventID string) string {
	return fmt.Sprintf("%sevent:%s", redisKeyPrefix, eventID)
}
