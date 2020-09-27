package timer

import (
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/dataobj"
)

type LocalTasksT struct {
	M map[int64]*Task
}

var Locals = &LocalTasksT{M: make(map[int64]*Task)}

func (lt *LocalTasksT) ReportTasks() []dataobj.ReportTask {
	ret := make([]dataobj.ReportTask, 0, len(lt.M))
	for id, t := range lt.M {
		rt := dataobj.ReportTask{Id: id, Clock: t.Clock}

		rt.Status = t.GetStatus()
		if rt.Status == "running" || rt.Status == "killing" {
			// intermediate state
			continue
		}

		rt.Stdout = t.GetStdout()
		rt.Stderr = t.GetStderr()

		stdoutLen := len(rt.Stdout)
		stderrLen := len(rt.Stderr)

		// 输出太长的话，截断，要不然把数据库撑爆了
		if stdoutLen > 65535 {
			start := stdoutLen - 65535
			rt.Stdout = rt.Stdout[start:]
		}

		if stderrLen > 65535 {
			start := stderrLen - 65535
			rt.Stderr = rt.Stderr[start:]
		}

		ret = append(ret, rt)
	}

	return ret
}

func (lt *LocalTasksT) GetTask(id int64) (*Task, bool) {
	t, found := lt.M[id]
	return t, found
}

func (lt *LocalTasksT) SetTask(t *Task) {
	lt.M[t.Id] = t
}

func (lt *LocalTasksT) AssignTask(at dataobj.AssignTask) {
	local, found := lt.GetTask(at.Id)
	if found {
		if local.Clock == at.Clock && local.Action == at.Action {
			// ignore repeat task
			return
		}

		local.Clock = at.Clock
		local.Action = at.Action
	} else {
		if at.Action == "kill" {
			// no process in local, no need kill
			return
		}
		local = &Task{
			Id:     at.Id,
			Clock:  at.Clock,
			Action: at.Action,
		}
		lt.SetTask(local)

		if local.doneBefore() {
			local.loadResult()
			return
		}
	}

	if local.Action == "kill" {
		local.SetStatus("killing")
		local.kill()
	} else if local.Action == "start" {
		local.SetStatus("running")
		local.start()
	} else {
		logger.Warningf("unknown action: %s of task %d", at.Action, at.Id)
	}
}

func (lt *LocalTasksT) Clean(assigned map[int64]struct{}) {
	del := make(map[int64]struct{})

	for id := range lt.M {
		if _, found := assigned[id]; !found {
			del[id] = struct{}{}
		}
	}

	for id := range del {
		// 远端已经不关注这个任务了，但是本地来看，任务还是running的
		// 可能是远端认为超时了，此时本地不能删除，仍然要继续上报
		if lt.M[id].GetStatus() == "running" {
			continue
		}

		lt.M[id].ResetBuff()
		cmd := lt.M[id].Cmd
		delete(lt.M, id)
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Release()
		}
	}
}
