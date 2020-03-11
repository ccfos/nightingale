package model

type TeamUser struct {
	TeamId  int64 `json:"team_id" xorm:"'team_id'"`
	UserId  int64 `json:"user_id" xorm:"'user_id'"`
	IsAdmin int   `json:"is_admin" xorm:"'is_admin'"`
}

func UserIdGetByTeamIds(teamIds []int64) ([]int64, error) {
	var objs []TeamUser
	err := DB["uic"].In("team_id", teamIds).Find(&objs)
	if err != nil {
		return nil, err
	}

	userIds := []int64{}
	for i := 0; i < len(objs); i++ {
		userIds = append(userIds, objs[i].UserId)
	}

	return userIds, nil
}
