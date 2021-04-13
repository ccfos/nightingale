package core

import (
	"fmt"

	"github.com/didi/nightingale/v4/src/common/client"
	"github.com/didi/nightingale/v4/src/common/dataobj"

	"github.com/toolkits/pkg/logger"
)

// Meta 从Server端获取任务元信息
func Meta(id int64) (script string, args string, account string, err error) {
	var resp dataobj.TaskMetaResponse
	err = client.GetCli("server").Call("Server.GetTaskMeta", id, &resp)
	if err != nil {
		logger.Error("rpc call Server.GetTaskMeta get error: ", err)
		client.CloseCli()
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
