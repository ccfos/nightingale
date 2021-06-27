package models

import "github.com/toolkits/pkg/logger"

type ClasspathFavorite struct {
	Id          int64
	ClasspathId int64
	UserId      int64
}

func (ClasspathFavorite) TableName() string {
	return "classpath_favorite"
}

func ClasspathFavoriteCount(where string, args ...interface{}) (int64, error) {
	num, err := DB.Where(where, args...).Count(new(ClasspathFavorite))
	if err != nil {
		logger.Errorf("mysql.error: count classpath_favorite(where=%s, args=%+v) fail: %v", where, args, err)
		return 0, internalServerError
	}
	return num, nil
}

func ClasspathFavoriteAdd(ClasspathId, userId int64) error {
	num, err := ClasspathFavoriteCount("user_id=? and classpath_id=?", userId, ClasspathId)
	if err != nil {
		return err
	}

	if num > 0 {
		// already exists
		return nil
	}

	obj := ClasspathFavorite{
		ClasspathId: ClasspathId,
		UserId:      userId,
	}

	return DBInsertOne(obj)
}

func ClasspathFavoriteDel(classpathId int64, userId int64) error {
	_, err := DB.Where("user_id=? and classpath_id=?", userId, classpathId).Delete(new(ClasspathFavorite))
	if err != nil {
		logger.Errorf("mysql.error: delete classpath_favorite fail: %v", err)
		return internalServerError
	}

	return nil
}

func ClasspathFavoriteGetClasspathIds(userId int64) ([]int64, error) {
	var ids []int64
	err := DB.Table(new(ClasspathFavorite)).Select("classpath_id").Where("user_id=?", userId).Find(&ids)
	if err != nil {
		logger.Errorf("mysql.error: query classpath_favorite fail: %v", err)
		return ids, internalServerError
	}

	return ids, nil
}
