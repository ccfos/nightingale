package cache

import (
	"sync"
)

type UserGroupMemberMap struct {
	sync.RWMutex
	Data map[int64]map[int64]struct{}
}

// groupid -> userid
var UserGroupMember = &UserGroupMemberMap{Data: make(map[int64]map[int64]struct{})}

func (m *UserGroupMemberMap) Get(id int64) (map[int64]struct{}, bool) {
	m.RLock()
	defer m.RUnlock()
	ids, exists := m.Data[id]
	return ids, exists
}

func (m *UserGroupMemberMap) Exists(gid, uid int64) bool {
	m.RLock()
	defer m.RUnlock()
	uidMap, exists := m.Data[gid]
	if !exists {
		return false
	}

	_, exists = uidMap[uid]
	return exists
}

func (m *UserGroupMemberMap) SetAll(data map[int64]map[int64]struct{}) {
	m.Lock()
	defer m.Unlock()
	m.Data = data
}
