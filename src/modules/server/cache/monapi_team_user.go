package cache

import (
	"sync"
)

type TeamUsersMap struct {
	sync.RWMutex
	Data map[int64][]int64
}

var TeamUsersCache *TeamUsersMap

func NewTeamUsersCache() *TeamUsersMap {
	return &TeamUsersMap{Data: make(map[int64][]int64)}
}

func (s *TeamUsersMap) GetBy(id int64) []int64 {
	s.RLock()
	defer s.RUnlock()

	return s.Data[id]
}

func (s *TeamUsersMap) GetByTeamIds(ids []int64) []int64 {
	s.RLock()
	defer s.RUnlock()
	m := make(map[int64]struct{})
	var userIds []int64

	for _, id := range ids {
		for _, uid := range s.Data[id] {
			m[uid] = struct{}{}
		}
	}

	for id, _ := range m {
		userIds = append(userIds, id)
	}

	return userIds
}

func (s *TeamUsersMap) SetAll(data map[int64][]int64) {
	s.Lock()
	defer s.Unlock()

	s.Data = data
	return
}
