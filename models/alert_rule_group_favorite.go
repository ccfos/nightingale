package models

import "github.com/toolkits/pkg/logger"

type AlertRuleGroupFavorite struct {
	Id      int64
	GroupId int64
	UserId  int64
}

func (AlertRuleGroupFavorite) TableName() string {
	return "alert_rule_group_favorite"
}

func AlertRuleGroupFavoriteCount(where string, args ...interface{}) (int64, error) {
	num, err := DB.Where(where, args...).Count(new(AlertRuleGroupFavorite))
	if err != nil {
		logger.Errorf("mysql.error: count alert_rule_group_favorite(where=%s, args=%+v) fail: %v", where, args, err)
		return 0, internalServerError
	}
	return num, nil
}

func AlertRuleGroupFavoriteAdd(groupId, userId int64) error {
	num, err := AlertRuleGroupFavoriteCount("user_id=? and group_id=?", userId, groupId)
	if err != nil {
		return err
	}

	if num > 0 {
		// already exists
		return nil
	}

	obj := AlertRuleGroupFavorite{
		GroupId: groupId,
		UserId:  userId,
	}

	return DBInsertOne(obj)
}

func AlertRuleGroupFavoriteDel(groupId, userId int64) error {
	_, err := DB.Where("user_id=? and group_id=?", userId, groupId).Delete(new(AlertRuleGroupFavorite))
	if err != nil {
		logger.Errorf("mysql.error: delete alert_rule_group_favorite fail: %v", err)
		return internalServerError
	}

	return nil
}

func AlertRuleGroupFavoriteGetGroupIds(userId int64) ([]int64, error) {
	var ids []int64
	err := DB.Table(new(AlertRuleGroupFavorite)).Select("group_id").Where("user_id=?", userId).Find(&ids)
	if err != nil {
		logger.Errorf("mysql.error: query alert_rule_group_favorite fail: %v", err)
		return ids, internalServerError
	}

	return ids, nil
}
