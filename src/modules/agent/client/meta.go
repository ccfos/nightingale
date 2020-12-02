package client

import (
	"fmt"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/dataobj"
)

// Meta 从Server端获取任务元信息
func Meta(id int64) (script string, args string, account string, err error) {
	var resp dataobj.TaskMetaResponse
	err = GetCli().Call("Scheduler.GetTaskMeta", id, &resp)
	if err != nil {
		return
	}

	if resp.Message != "" {
		logger.Error("rpc call Scheduler.GetTaskMeta get error message: ", resp.Message)
		err = fmt.Errorf(resp.Message)
		return
	}

	script = resp.Script
	args = resp.Args
	account = resp.Account
	return
}
