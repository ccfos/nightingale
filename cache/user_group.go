package cache

import (
	"sync"

	"github.com/didi/nightingale/v5/models"
)

type UserGroupMap struct {
	sync.RWMutex
	Data map[int64]*models.UserGroup
}

var UserGroupCache = &UserGroupMap{Data: make(map[int64]*models.UserGroup)}

func (s *UserGroupMap) GetBy(id int64) *models.UserGroup {
	s.RLock()
	defer s.RUnlock()

	return s.Data[id]
}

func (s *UserGroupMap) GetByIds(ids []int64) []*models.UserGroup {
	s.RLock()
	defer s.RUnlock()
	var userGroups []*models.UserGroup
	for _, id := range ids {
		if s.Data[id] == nil {
			continue
		}
		userGroups = append(userGroups, s.Data[id])
	}

	return userGroups
}

func (s *UserGroupMap) SetAll(userGroups map[int64]*models.UserGroup) {
	s.Lock()
	defer s.Unlock()
	s.Data = userGroups
}
