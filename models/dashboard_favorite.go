package models

import "github.com/toolkits/pkg/logger"

type DashboardFavorite struct {
	Id          int64
	DashboardId int64
	UserId      int64
}

func (DashboardFavorite) TableName() string {
	return "dashboard_favorite"
}

func DashboardFavoriteCount(where string, args ...interface{}) (int64, error) {
	num, err := DB.Where(where, args...).Count(new(DashboardFavorite))
	if err != nil {
		logger.Errorf("mysql.error: count dashboard_favorite(where=%s, args=%+v) fail: %v", where, args, err)
		return 0, internalServerError
	}
	return num, nil
}

func DashboardFavoriteAdd(dashboardId, userId int64) error {
	num, err := DashboardFavoriteCount("user_id=? and dashboard_id=?", userId, dashboardId)
	if err != nil {
		return err
	}

	if num > 0 {
		// already exists
		return nil
	}

	obj := DashboardFavorite{
		DashboardId: dashboardId,
		UserId:      userId,
	}

	return DBInsertOne(obj)
}

func DashboardFavoriteDel(dashboardId, userId int64) error {
	_, err := DB.Where("user_id=? and dashboard_id=?", userId, dashboardId).Delete(new(DashboardFavorite))
	if err != nil {
		logger.Errorf("mysql.error: delete dashboard_favorite fail: %v", err)
		return internalServerError
	}

	return nil
}

func DashboardFavoriteGetDashboardIds(userId int64) ([]int64, error) {
	var ids []int64
	err := DB.Table(new(DashboardFavorite)).Select("dashboard_id").Where("user_id=?", userId).Find(&ids)
	if err != nil {
		logger.Errorf("mysql.error: query dashboard_favorite fail: %v", err)
		return ids, internalServerError
	}

	return ids, nil
}
