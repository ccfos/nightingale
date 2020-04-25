package procs

import (
	"github.com/didi/nightingale/src/model"
)

var (
	Procs      = make(map[string]*model.ProcCollect)
	Schedulers = make(map[string]*ProcScheduler)
)

func DelNoProcCollect(newCollect map[string]*model.ProcCollect) {
	for currKey, currProc := range Procs {
		newProc, ok := newCollect[currKey]
		if !ok || currProc.LastUpdated != newProc.LastUpdated {
			deleteProc(currKey)
		}
	}
}

func AddNewProcCollect(newCollect map[string]*model.ProcCollect) {
	for target, newProc := range newCollect {
		if _, ok := Procs[target]; ok && newProc.LastUpdated == Procs[target].LastUpdated {
			continue
		}

		Procs[target] = newProc
		sch := NewProcScheduler(newProc)
		Schedulers[target] = sch
		sch.Schedule()
	}
}

func deleteProc(key string) {
	if v, ok := Schedulers[key]; ok {
		v.Stop()
		delete(Schedulers, key)
	}
	delete(Procs, key)
}
