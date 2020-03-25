package procs

import (
	"github.com/didi/nightingale/src/model"
)

var (
	Procs              = make(map[string]*model.ProcCollect)
	ProcsWithScheduler = make(map[string]*ProcScheduler)
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
		ProcsWithScheduler[target] = sch
		sch.Schedule()
	}
}

func deleteProc(key string) {
	v, ok := ProcsWithScheduler[key]
	if ok {
		v.Stop()
		delete(ProcsWithScheduler, key)
	}
	delete(Procs, key)
}
