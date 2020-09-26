package redisc

import (
	"encoding/json"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/dataobj"
)

func Pop(count int, queue string) []*dataobj.Message {
	var ret []*dataobj.Message

	rc := RedisConnPool.Get()
	defer rc.Close()

	for i := 0; i < count; i++ {
		reply, err := redis.String(rc.Do("RPOP", queue))
		if err != nil {
			if err != redis.ErrNil {
				logger.Errorf("rpop queue:%s failed, err: %v", queue, err)
			}
			break
		}

		if reply == "" || reply == "nil" {
			continue
		}

		var message dataobj.Message
		err = json.Unmarshal([]byte(reply), &message)
		if err != nil {
			logger.Errorf("unmarshal message failed, err: %v, redis reply: %v", err, reply)
			continue
		}

		ret = append(ret, &message)
	}

	return ret
}
