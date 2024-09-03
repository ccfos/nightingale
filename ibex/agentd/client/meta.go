package client

import (
	"fmt"
	"log"

	"github.com/ccfos/nightingale/v6/ibex/types"
)

// Meta 从Server端获取任务元信息
func Meta(id int64) (script string, args string, account string, stdin string, err error) {
	var resp types.TaskMetaResponse
	err = GetCli().Call("Server.GetTaskMeta", id, &resp)
	if err != nil {
		log.Println("E: rpc call Server.GetTaskMeta:", err)
		CloseCli()
		return
	}

	if resp.Message != "" {
		log.Println("E: rpc call Server.GetTaskMeta:", resp.Message)
		err = fmt.Errorf(resp.Message)
		return
	}

	script = resp.Script
	args = resp.Args
	account = resp.Account
	stdin = resp.Stdin
	return
}
