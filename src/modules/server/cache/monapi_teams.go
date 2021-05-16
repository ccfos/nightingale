package cache

import (
	"strconv"
	"strings"
	"sync"

	"github.com/didi/nightingale/v4/src/models"
)

type TeamMap struct {
	sync.RWMutex
	Data map[int64]*models.Team
}

var TeamCache *TeamMap

func NewTeamCache() *TeamMap {
	return &TeamMap{Data: make(map[int64]*models.Team)}
}

func (s *TeamMap) GetBy(id int64) *models.Team {
	s.RLock()
	defer s.RUnlock()

	return s.Data[id]
}

func (s *TeamMap) GetByIds(ids []int64) []*models.Team {
	s.RLock()
	defer s.RUnlock()
	var objs []*models.Team
	for _, id := range ids {
		if s.Data[id] == nil {
			continue
		}
		objs = append(objs, s.Data[id])
	}

	return objs
}

func (s *TeamMap) SetAll(objs map[int64]*models.Team) {
	s.Lock()
	defer s.Unlock()

	s.Data = objs
	return
}

func (s *TeamMap) GetTeamNamesByIds(ids string) []string {
	var names []string
	ids = strings.Replace(ids, "[", "", -1)
	ids = strings.Replace(ids, "]", "", -1)
	idsStrArr := strings.Split(ids, ",")

	teamIds := []int64{}
	for _, teamId := range idsStrArr {
		id, _ := strconv.ParseInt(teamId, 10, 64)
		teamIds = append(teamIds, id)
	}

	objs := s.GetByIds(teamIds)
	for _, obj := range objs {
		names = append(names, obj.Name)
	}

	return names
}
