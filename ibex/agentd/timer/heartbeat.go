package timer

import (
	"context"
	"log"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/agentd/client"
	"github.com/ccfos/nightingale/v6/ibex/agentd/config"
	"github.com/ccfos/nightingale/v6/ibex/types"
)

func Heartbeat(ctx context.Context) {
	interval := time.Duration(config.C.Heartbeat.Interval) * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			heartbeat()
		}
	}
}

func heartbeat() {
	ident, err := config.C.GetHost()
	if err != nil {
		log.Println("E: GetHost fail:", err)
		return
	}

	req := types.ReportRequest{
		Ident:       ident,
		ReportTasks: Locals.ReportTasks(),
	}

	var resp types.ReportResponse
	err = client.GetCli().Call("Server.Report", req, &resp)
	if err != nil {
		log.Println("E: rpc call Server.Report fail:", err)
		client.CloseCli()
		return
	}

	if resp.Message != "" {
		log.Println("E: error from server:", resp.Message)
		return
	}

	assigned := make(map[int64]struct{})

	if resp.AssignTasks != nil {
		count := len(resp.AssignTasks)
		for i := 0; i < count; i++ {
			at := resp.AssignTasks[i]
			assigned[at.Id] = struct{}{}
			Locals.AssignTask(at)
		}
	}

	if len(assigned) > 0 {
		log.Println("D: assigned tasks:", mapKeys(assigned))
	}

	Locals.Clean(assigned)
}

func mapKeys(m map[int64]struct{}) []int64 {
	lst := make([]int64, 0, len(m))
	for k := range m {
		lst = append(lst, k)
	}
	return lst
}
