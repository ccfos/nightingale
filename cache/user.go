package cache

import (
	"sync"

	"github.com/didi/nightingale/v5/models"
)

type UserMap struct {
	sync.RWMutex
	Data map[int64]*models.User
}

var UserCache = &UserMap{Data: make(map[int64]*models.User)}

func (s *UserMap) GetBy(id int64) *models.User {
	s.RLock()
	defer s.RUnlock()

	return s.Data[id]
}

func (s *UserMap) GetByIds(ids []int64) []*models.User {
	s.RLock()
	defer s.RUnlock()
	var users []*models.User
	for _, id := range ids {
		if s.Data[id] == nil {
			continue
		}
		users = append(users, s.Data[id])
	}

	return users
}

func (s *UserMap) SetAll(users map[int64]*models.User) {
	s.Lock()
	defer s.Unlock()
	s.Data = users
}
