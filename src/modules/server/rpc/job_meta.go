package rpc

import (
	"fmt"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/models"
)

// GetTaskMeta 获取任务元信息，自带缓存
func (*Server) GetTaskMeta(id int64, resp *dataobj.TaskMetaResponse) error {
	meta, err := models.TaskMetaGetByID(id)
	if err != nil {
		resp.Message = err.Error()
		return nil
	}

	if meta == nil {
		resp.Message = fmt.Sprintf("task %d not found", id)
		return nil
	}

	resp.Script = meta.Script
	resp.Args = meta.Args
	resp.Account = meta.Account
	return nil
}
