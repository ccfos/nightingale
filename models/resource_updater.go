package models

import (
	"time"

	cmap "github.com/orcaman/concurrent-map"
	"github.com/toolkits/pkg/logger"
)

type AliasTime struct {
	Alias string
	Time  int64
}

var AliasMapper = cmap.New()

func UpdateAlias() error {
	mapper := AliasMapper.Items()
	if len(mapper) == 0 {
		logger.Warning("alias mapper is nil, no points push?")
		return nil
	}

	now := time.Now().Unix()

	// 先清理数据，只保留最近15s的数据
	for key, at := range mapper {
		if at.(*AliasTime).Time < now-15 {
			AliasMapper.Remove(key)
		}
	}

	// 从数据库获取所有的ident->alias对应关系
	dbmap, err := ResourceAliasMapper()
	if err != nil {
		logger.Warningf("ResourceAliasMapper fail: %v", err)
		return err
	}

	// 从内存里拿到最新的ident->alias对应关系
	upmap := AliasMapper.Items()

	for key, upval := range upmap {
		dbval, has := dbmap[key]
		if !has {
			// 数据库里没有，写入
			err = DBInsertOne(Resource{
				Ident: key,
				Alias: upval.(*AliasTime).Alias,
			})
			if err != nil {
				logger.Errorf("mysql.error: insert resource(ident=%s, alias=%s) fail: %v", key, upval.(*AliasTime).Alias, err)
			} else {
				// 新资源，默认绑定到id为1的classpath，方便用户管理
				if err = ClasspathResourceAdd(1, key); err != nil {
					logger.Errorf("bind resource(%s) to classpath(1) fail: %v", key, err)
				}
			}

			continue
		}

		if upval.(*AliasTime).Alias != dbval {
			// alias 的值与 DB 中不同，更新
			_, err = DB.Exec("UPDATE resource SET alias=? WHERE ident=?", upval.(*AliasTime).Alias, key)
			if err != nil {
				logger.Errorf("mysql.error: update resource(ident=%s, alias=%s) fail: %v", key, upval.(*AliasTime).Alias, err)
			}
		}
	}

	return nil
}
