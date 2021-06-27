package models

import "github.com/toolkits/pkg/logger"

type ClasspathResource struct {
	Id          int64
	ClasspathId int64
	ResIdent    string
}

func (ClasspathResource) TableName() string {
	return "classpath_resource"
}

func ClasspathResourceCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(ClasspathResource))
	if err != nil {
		logger.Errorf("mysql.error: count classpath_resource fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func ClasspathResourceGets(where string, args ...interface{}) ([]ClasspathResource, error) {
	var objs []ClasspathResource

	err := DB.Where(where, args...).Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: get classpath_resources fail: %v", err)
		return objs, internalServerError
	}

	return objs, nil
}

func ClasspathResourceGetAll() ([]ClasspathResource, error) {
	var objs []ClasspathResource

	err := DB.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: get classpath_resources fail: %v", err)
		return objs, internalServerError
	}

	return objs, nil
}

func ClasspathResourceAdd(classpathId int64, resIdent string) error {
	num, err := ClasspathResourceCount("classpath_id=? and res_ident=?", classpathId, resIdent)
	if err != nil {
		return err
	}

	if num > 0 {
		// already exists
		return nil
	}

	obj := ClasspathResource{
		ClasspathId: classpathId,
		ResIdent:    resIdent,
	}

	return DBInsertOne(obj)
}

func ClasspathResourceDel(classpathId int64, idents []string) error {
	if len(idents) == 0 {
		return nil
	}

	_, err := DB.Where("classpath_id=?", classpathId).In("res_ident", idents).Delete(new(ClasspathResource))
	if err != nil {
		logger.Errorf("mysql.error: delete classpath_resource fail: %v", err)
		return internalServerError
	}

	return nil
}

// 如果发现资源没有在all这个classpath下，就把它加进来
func BindOrphanToPresetClasspath() {
	sql := "insert into classpath_resource(classpath_id, res_ident) select 1, ident from resource where ident not in (select res_ident from classpath_resource)"
	ret, err := DB.Exec(sql)
	if err != nil {
		logger.Errorf("mysql.error: addOrphanToPresetClasspath fail: %v", err)
		return
	}

	num, err := ret.RowsAffected()
	if err != nil {
		logger.Errorf("mysql.error: cannot load rows affected: %v", err)
		return
	}

	if num > 0 {
		logger.Warningf("we found %d orphan resources", num)
	}
}
