package models

import (
	"strings"

	"github.com/didi/nightingale/v5/pkg/istr"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type Resource struct {
	Id           int64   `json:"id"`
	Ident        string  `json:"ident"`
	Alias        string  `json:"alias"`
	Tags         string  `json:"tags"`
	Note         string  `json:"note"`
	MuteBtime    int64   `json:"mute_btime"`
	MuteEtime    int64   `json:"mute_etime"`
	ClasspathIds []int64 `xorm:"-" json:"classpath_ids"`
}

func (r *Resource) Del() error {
	session := DB.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM classpath_resource WHERE res_ident=?", r.Ident); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM resource WHERE id=?", r.Id); err != nil {
		return err
	}

	return session.Commit()
}

func ResourceTotalByClasspathId(classpathIds []int64, query string) (int64, error) {
	if len(classpathIds) == 0 {
		return 0, nil
	}

	q := "%" + query + "%"
	num, err := DB.Where("ident in (select res_ident from classpath_resource where classpath_id in ("+str.IdsString(classpathIds)+")) and (ident like ? or alias like ? or tags like ? or note like ?)", q, q, q, q).Count(new(Resource))
	if err != nil {
		logger.Errorf("mysql.error count resource in classpath(id=%v) query=%s fail: %v", classpathIds, query, err)
		return 0, internalServerError
	}
	return num, nil
}

func ResourceGetsByClasspathId(classpathIds []int64, query string, limit, offset int) ([]Resource, error) {
	if len(classpathIds) == 0 {
		return []Resource{}, nil
	}

	q := "%" + query + "%"

	var objs []Resource
	err := DB.Where("ident in (select res_ident from classpath_resource where classpath_id in ("+str.IdsString(classpathIds)+")) and (ident like ? or alias like ? or tags like ? or note like ?)", q, q, q, q).OrderBy("ident").Limit(limit, offset).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error query resource in classpath(id=%d) query=%s fail: %v", classpathIds, query, err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Resource{}, nil
	}

	return objs, nil
}

// 这个sql写法很简单，只是不知道性能是否会是问题
func ResourceTotalByClasspathQuery(qpaths []int64, qres string) (int64, error) {
	if len(qpaths) == 0 {
		return ResourceTotal(qres)
	}

	num, err := DB.Where("ident in (select res_ident from classpath_resource where classpath_id in ("+str.IdsString(qpaths)+")) and (ident like ? or alias like ?)", "%"+qres+"%", "%"+qres+"%").Count(new(Resource))
	if err != nil {
		logger.Errorf("mysql.error ResourceTotalByClasspathQuery fail: %v", err)
		return 0, internalServerError
	}
	return num, nil
}

func ResourceGetsByClasspathQuery(qpaths []int64, qres string, limit, offset int) ([]Resource, error) {
	if len(qpaths) == 0 {
		return ResourceGets(qres, limit, offset)
	}

	var objs []Resource
	err := DB.Where("ident in (select res_ident from classpath_resource where classpath_id in ("+str.IdsString(qpaths)+")) and (ident like ? or alias like ?)", "%"+qres+"%", "%"+qres+"%").OrderBy("ident").Limit(limit, offset).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error ResourceGetsByClasspathQuery fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Resource{}, nil
	}

	return objs, nil
}

// ResourceTotal query resource only
func ResourceTotal(query string) (int64, error) {
	num, err := DB.Where("ident like ? or alias like ?", "%"+query+"%", "%"+query+"%").Count(new(Resource))
	if err != nil {
		logger.Errorf("mysql.error query(%s) resource fail: %v", query, err)
		return 0, internalServerError
	}

	return num, nil
}

// ResourceGets query resource only
func ResourceGets(query string, limit, offset int) ([]Resource, error) {
	var objs []Resource

	err := DB.Where("ident like ? or alias like ?", "%"+query+"%", "%"+query+"%").OrderBy("ident").Limit(limit, offset).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error query resource fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Resource{}, nil
	}

	return objs, nil
}

func ResourceGetAll() ([]Resource, error) {
	var objs []Resource

	err := DB.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error query resource fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Resource{}, nil
	}

	return objs, nil
}

func ResourceCleanExpire() error {
	_, err := DB.Exec("UPDATE resource SET mute_btime = 0, mute_etime = 0 WHERE mute_etime < unix_timestamp(now())")
	if err != nil {
		logger.Errorf("mysql.error: ResourceCleanExpire fail: %v", err)
	}
	return err
}

// ResourceAliasMapper 返回map，key是ident，value是alias
func ResourceAliasMapper() (map[string]string, error) {
	var objs []Resource
	err := DB.Cols("ident", "alias").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: ResourceAliasMapper fail: %v", err)
		return nil, err
	}

	count := len(objs)
	if count == 0 {
		return map[string]string{}, nil
	}

	mapper := make(map[string]string, count)
	for i := 0; i < count; i++ {
		mapper[objs[i].Ident] = objs[i].Alias
	}

	return mapper, nil
}

func ResourceGet(where string, args ...interface{}) (*Resource, error) {
	var obj Resource
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: get resource(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func ResourceUpdateNote(ids []int64, note string) error {
	_, err := DB.Exec("UPDATE resource SET note=? WHERE id in ("+str.IdsString(ids)+")", note)
	if err != nil {
		logger.Errorf("mysql.error: update resource note fail: %s", err)
		return internalServerError
	}
	return nil
}

// 资源tags这个更新，一定要认真校验tags格式，后面会把这个tags附到监控数据里
func ResourceUpdateTags(ids []int64, tags string) error {
	tags = strings.TrimSpace(tags)

	if tags != "" {
		// 不为空才值得去校验，空的字符串，没啥好校验的
		arr := strings.Fields(tags)
		cnt := len(arr)
		for i := 0; i < cnt; i++ {
			tagarr := strings.Split(arr[i], "=")
			if len(tagarr) != 2 {
				return _e("Tags(%s) invalid", arr[i])
			}

			if istr.SampleKeyInvalid(tagarr[0]) {
				return _e("Tags(%s) invalid", arr[i])
			}
		}
	}

	_, err := DB.Exec("UPDATE resource SET tags=? WHERE id in ("+str.IdsString(ids)+")", tags)
	if err != nil {
		logger.Errorf("mysql.error: update resource tags fail: %s", err)
		return internalServerError
	}

	return nil
}

func ResourceUpdateMute(ids []int64, btime, etime int64) error {
	_, err := DB.Exec("UPDATE resource SET mute_btime=?, mute_etime=? WHERE id in ("+str.IdsString(ids)+")", btime, etime)
	if err != nil {
		logger.Errorf("mysql.error: update resource mute fail: %s", err)
		return internalServerError
	}
	return nil
}
