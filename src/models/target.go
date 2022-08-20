package models

import (
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Target struct {
	Id       int64             `json:"id" gorm:"primaryKey"`
	GroupId  int64             `json:"group_id"`
	GroupObj *BusiGroup        `json:"group_obj" gorm:"-"`
	Cluster  string            `json:"cluster"`
	Ident    string            `json:"ident"`
	Note     string            `json:"note"`
	Tags     string            `json:"-"`
	TagsJSON []string          `json:"tags" gorm:"-"`
	TagsMap  map[string]string `json:"-" gorm:"-"` // internal use, append tags to series
	UpdateAt int64             `json:"update_at"`

	TargetUp    float64 `json:"target_up" gorm:"-"`
	LoadPerCore float64 `json:"load_per_core" gorm:"-"`
	MemUtil     float64 `json:"mem_util" gorm:"-"`
	DiskUtil    float64 `json:"disk_util" gorm:"-"`
}

func (t *Target) TableName() string {
	return "target"
}

func (t *Target) Add() error {
	obj, err := TargetGet("ident = ?", t.Ident)
	if err != nil {
		return err
	}

	if obj == nil {
		return Insert(t)
	}

	if obj.Cluster != t.Cluster {
		return DB().Model(&Target{}).Where("ident = ?", t.Ident).Updates(map[string]interface{}{
			"cluster":   t.Cluster,
			"update_at": t.UpdateAt,
		}).Error
	}

	return nil
}

func (t *Target) FillGroup(cache map[int64]*BusiGroup) error {
	if t.GroupId <= 0 {
		return nil
	}

	bg, has := cache[t.GroupId]
	if has {
		t.GroupObj = bg
		return nil
	}

	bg, err := BusiGroupGetById(t.GroupId)
	if err != nil {
		return errors.WithMessage(err, "failed to get busi group")
	}

	t.GroupObj = bg
	cache[t.GroupId] = bg
	return nil
}

func TargetStatistics(cluster string) (*Statistics, error) {
	session := DB().Model(&Target{}).Select("count(*) as total", "max(update_at) as last_updated")
	if cluster != "" {
		session = session.Where("cluster = ?", cluster)
	}

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func TargetDel(idents []string) error {
	if len(idents) == 0 {
		panic("idents empty")
	}
	return DB().Where("ident in ?", idents).Delete(new(Target)).Error
}

func buildTargetWhere(bgid int64, clusters []string, query string) *gorm.DB {
	session := DB().Model(&Target{})

	if bgid >= 0 {
		session = session.Where("group_id=?", bgid)
	}

	if len(clusters) > 0 {
		session = session.Where("cluster in ?", clusters)
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			q := "%" + arr[i] + "%"
			session = session.Where("ident like ? or note like ? or tags like ?", q, q, q)
		}
	}

	return session
}

func TargetTotalCount() (int64, error) {
	return Count(DB().Model(new(Target)))
}

func TargetTotal(bgid int64, clusters []string, query string) (int64, error) {
	return Count(buildTargetWhere(bgid, clusters, query))
}

func TargetGets(bgid int64, clusters []string, query string, limit, offset int) ([]*Target, error) {
	var lst []*Target
	err := buildTargetWhere(bgid, clusters, query).Order("ident").Limit(limit).Offset(offset).Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].TagsJSON = strings.Fields(lst[i].Tags)
		}
	}
	return lst, err
}

func TargetGetsByCluster(cluster string) ([]*Target, error) {
	session := DB().Model(&Target{})
	if cluster != "" {
		session = session.Where("cluster = ?", cluster)
	}

	var lst []*Target
	err := session.Find(&lst).Error
	return lst, err
}

func TargetUpdateNote(idents []string, note string) error {
	return DB().Model(&Target{}).Where("ident in ?", idents).Updates(map[string]interface{}{
		"note":      note,
		"update_at": time.Now().Unix(),
	}).Error
}

func TargetUpdateBgid(idents []string, bgid int64, clearTags bool) error {
	fields := map[string]interface{}{
		"group_id":  bgid,
		"update_at": time.Now().Unix(),
	}

	if clearTags {
		fields["tags"] = ""
	}

	return DB().Model(&Target{}).Where("ident in ?", idents).Updates(fields).Error
}

func TargetGet(where string, args ...interface{}) (*Target, error) {
	var lst []*Target
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].TagsJSON = strings.Fields(lst[0].Tags)

	return lst[0], nil
}

func TargetGetById(id int64) (*Target, error) {
	return TargetGet("id = ?", id)
}

func TargetGetByIdent(ident string) (*Target, error) {
	return TargetGet("ident = ?", ident)
}

func TargetGetTags(idents []string) ([]string, error) {
	if len(idents) == 0 {
		return []string{}, nil
	}

	var arr []string
	err := DB().Model(new(Target)).Where("ident in ?", idents).Select("distinct(tags) as tags").Pluck("tags", &arr).Error
	if err != nil {
		return nil, err
	}

	cnt := len(arr)
	if cnt == 0 {
		return []string{}, nil
	}

	set := make(map[string]struct{})
	for i := 0; i < cnt; i++ {
		tags := strings.Fields(arr[i])
		for j := 0; j < len(tags); j++ {
			set[tags[j]] = struct{}{}
		}
	}

	cnt = len(set)
	ret := make([]string, 0, cnt)
	for key := range set {
		ret = append(ret, key)
	}

	sort.Strings(ret)

	return ret, err
}

func (t *Target) AddTags(tags []string) error {
	for i := 0; i < len(tags); i++ {
		if -1 == strings.Index(t.Tags, tags[i]+" ") {
			t.Tags += tags[i] + " "
		}
	}

	arr := strings.Fields(t.Tags)
	sort.Strings(arr)

	return DB().Model(t).Updates(map[string]interface{}{
		"tags":      strings.Join(arr, " ") + " ",
		"update_at": time.Now().Unix(),
	}).Error
}

func (t *Target) DelTags(tags []string) error {
	for i := 0; i < len(tags); i++ {
		t.Tags = strings.ReplaceAll(t.Tags, tags[i]+" ", "")
	}

	return DB().Model(t).Updates(map[string]interface{}{
		"tags":      t.Tags,
		"update_at": time.Now().Unix(),
	}).Error
}

func TargetIdents(ids []int64) ([]string, error) {
	var ret []string

	if len(ids) == 0 {
		return ret, nil
	}

	err := DB().Model(&Target{}).Where("id in ?", ids).Pluck("ident", &ret).Error
	return ret, err
}

func TargetIds(idents []string) ([]int64, error) {
	var ret []int64

	if len(idents) == 0 {
		return ret, nil
	}

	err := DB().Model(&Target{}).Where("ident in ?", idents).Pluck("id", &ret).Error
	return ret, err
}

func IdentsFilter(idents []string, where string, args ...interface{}) ([]string, error) {
	var arr []string
	if len(idents) == 0 {
		return arr, nil
	}

	err := DB().Model(&Target{}).Where("ident in ?", idents).Where(where, args...).Pluck("ident", &arr).Error
	return arr, err
}
