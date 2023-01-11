package engine

import "github.com/didi/nightingale/v5/src/server/common/sender"

type MessageSender interface {
	sender.DingtalkMessage
}
