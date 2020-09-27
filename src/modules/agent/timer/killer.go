package timer

import (
	"fmt"
	"strings"

	"github.com/toolkits/pkg/sys"

	"github.com/didi/nightingale/src/modules/agent/config"
)

func KillProcessByTaskID(id int64) error {
	dir := strings.TrimRight(config.Config.Job.MetaDir, "/")
	arr := strings.Split(dir, "/")
	lst := arr[len(arr)-1]
	return sys.KillProcessByCmdline(fmt.Sprintf("%s/%d/script", lst, id))
}
