package cache

import (
	"strconv"
	"strings"
	"sync"

	"github.com/didi/nightingale/v4/src/models"
)

type UserMap struct {
	sync.RWMutex
	Data map[int64]*models.User
}

var UserCache *UserMap

func NewUserCache() *UserMap {
	return &UserMap{Data: make(map[int64]*models.User)}
}

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
	return
}

func (s *UserMap) GetUsernamesByIds(ids string) []string {
	var names []string
	ids = strings.Replace(ids, "[", "", -1)
	ids = strings.Replace(ids, "]", "", -1)
	idsStrArr := strings.Split(ids, ",")

	userIds := []int64{}
	for _, userId := range idsStrArr {
		id, _ := strconv.ParseInt(userId, 10, 64)
		userIds = append(userIds, id)
	}

	users := s.GetByIds(userIds)
	for _, user := range users {
		names = append(names, user.Username)
	}

	return names
}
