package redisc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/dataobj"
)

func lpush(queue, message string) error {
	rc := RedisConnPool.Get()
	defer rc.Close()
	_, err := rc.Do("LPUSH", queue, message)
	if err != nil {
		logger.Errorf("LPUSH %s fail, message:%s, error:%v", queue, message, err)
	}
	return err
}

// Write LPUSH message to redis
func Write(data *dataobj.Message, queue string) error {
	if data == nil {
		return fmt.Errorf("message is nil")
	}

	data.Tos = removeEmptyString(data.Tos)

	bs, err := json.Marshal(data)
	if err != nil {
		logger.Errorf("marshal message failed, message: %+v, err: %v", data, err)
		return err
	}

	logger.Debugf("write message to queue, message:%+v, queue:%s", data, queue)
	return lpush(queue, string(bs))
}

func removeEmptyString(s []string) []string {
	cnt := len(s)
	ss := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		if strings.TrimSpace(s[i]) == "" {
			continue
		}

		ss = append(ss, s[i])
	}

	return ss
}
