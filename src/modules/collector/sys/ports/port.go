package ports

import (
	"github.com/didi/nightingale/src/model"
)

var (
	Ports              = make(map[int]*model.PortCollect)
	PortsWithScheduler = make(map[int]*PortScheduler)
)

func DelNoPortCollect(newCollect map[int]*model.PortCollect) {
	for currKey, currPort := range Ports {
		newPort, ok := newCollect[currKey]
		if !ok || currPort.LastUpdated != newPort.LastUpdated {
			deletePort(currKey)
		}
	}
}

func AddNewPortCollect(newCollect map[int]*model.PortCollect) {
	for target, newPort := range newCollect {
		if _, ok := Ports[target]; ok && newPort.LastUpdated == Ports[target].LastUpdated {
			continue
		}

		Ports[target] = newPort
		sch := NewPortScheduler(newPort)
		PortsWithScheduler[target] = sch
		sch.Schedule()
	}
}

func deletePort(key int) {
	v, ok := PortsWithScheduler[key]
	if ok {
		v.Stop()
		delete(PortsWithScheduler, key)
	}
	delete(Ports, key)
}
