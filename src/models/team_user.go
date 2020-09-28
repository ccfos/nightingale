package models

type TeamUser struct {
	TeamId  int64 `json:"team_id" xorm:"'team_id'"`
	UserId  int64 `json:"user_id" xorm:"'user_id'"`
	IsAdmin int   `json:"is_admin" xorm:"'is_admin'"`
}

func TeamMembers(tid int64, isAdmin int) ([]User, error) {
	ids, err := UserIdsByTeamIds([]int64{tid}, isAdmin)
	if err != nil {
		return nil, err
	}

	if ids == nil || len(ids) == 0 {
		return []User{}, nil
	}

	return UserGetByIds(ids)
}

func TeamIdsByUserId(uid int64, isAdmin ...int) ([]int64, error) {
	session := DB["rdb"].Table("team_user").Select("team_id").Where("user_id=?", uid)
	if len(isAdmin) > 0 {
		session = session.Where("is_admin=?", isAdmin[0])
	}

	var ids []int64
	err := session.Find(&ids)
	return ids, err
}

func UserIdsByTeamIds(tids []int64, isAdmin ...int) ([]int64, error) {
	if len(tids) == 0 {
		return []int64{}, nil
	}

	session := DB["rdb"].Table("team_user").Select("user_id").In("team_id", tids)
	if len(isAdmin) > 0 {
		session = session.Where("is_admin=?", isAdmin[0])
	}

	var ids []int64
	err := session.Find(&ids)
	return ids, err
}

func TeamHasMember(tid, uid int64, isAdmin ...int) (bool, error) {
	session := DB["rdb"].Where("team_id=? and user_id=?", tid, uid)
	if len(isAdmin) > 0 {
		session = session.Where("is_admin=?", isAdmin[0])
	}

	cnt, err := session.Count(new(TeamUser))
	return cnt > 0, err
}
