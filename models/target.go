package models

import (
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type Target struct {
	Id       int64             `json:"id" gorm:"primaryKey"`
	GroupId  int64             `json:"group_id"`
	GroupObj *BusiGroup        `json:"group_obj" gorm:"-"`
	Ident    string            `json:"ident"`
	Note     string            `json:"note"`
	Tags     string            `json:"-"`
	TagsJSON []string          `json:"tags" gorm:"-"`
	TagsMap  map[string]string `json:"-" gorm:"-"` // internal use, append tags to series
	UpdateAt int64             `json:"update_at"`

	UnixTime   int64   `json:"unixtime" gorm:"-"`
	Offset     int64   `json:"offset" gorm:"-"`
	TargetUp   float64 `json:"target_up" gorm:"-"`
	MemUtil    float64 `json:"mem_util" gorm:"-"`
	CpuNum     int     `json:"cpu_num" gorm:"-"`
	CpuUtil    float64 `json:"cpu_util" gorm:"-"`
	OS         string  `json:"os" gorm:"-"`
	Arch       string  `json:"arch" gorm:"-"`
	RemoteAddr string  `json:"remote_addr" gorm:"-"`
}

func (t *Target) TableName() string {
	return "target"
}

func (t *Target) FillGroup(ctx *ctx.Context, cache map[int64]*BusiGroup) error {
	if t.GroupId <= 0 {
		return nil
	}

	bg, has := cache[t.GroupId]
	if has {
		t.GroupObj = bg
		return nil
	}

	bg, err := BusiGroupGetById(ctx, t.GroupId)
	if err != nil {
		return errors.WithMessage(err, "failed to get busi group")
	}

	t.GroupObj = bg
	cache[t.GroupId] = bg
	return nil
}

func TargetStatistics(ctx *ctx.Context) (*Statistics, error) {
	var stats []*Statistics
	err := DB(ctx).Model(&Target{}).Select("count(*) as total", "max(update_at) as last_updated").Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func TargetDel(ctx *ctx.Context, idents []string) error {
	if len(idents) == 0 {
		panic("idents empty")
	}
	return DB(ctx).Where("ident in ?", idents).Delete(new(Target)).Error
}

func buildTargetWhere(ctx *ctx.Context, bgid int64, dsIds []int64, query string) *gorm.DB {
	session := DB(ctx).Model(&Target{})

	if bgid >= 0 {
		session = session.Where("group_id=?", bgid)
	}

	if len(dsIds) > 0 {
		session = session.Where("datasource_id in ?", dsIds)
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

func TargetTotalCount(ctx *ctx.Context) (int64, error) {
	return Count(DB(ctx).Model(new(Target)))
}

func TargetTotal(ctx *ctx.Context, bgid int64, dsIds []int64, query string) (int64, error) {
	return Count(buildTargetWhere(ctx, bgid, dsIds, query))
}

func TargetGets(ctx *ctx.Context, bgid int64, dsIds []int64, query string, limit, offset int) ([]*Target, error) {
	var lst []*Target
	err := buildTargetWhere(ctx, bgid, dsIds, query).Order("ident").Limit(limit).Offset(offset).Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].TagsJSON = strings.Fields(lst[i].Tags)
		}
	}
	return lst, err
}

// 根据 groupids, tags, hosts 查询 targets
func TargetGetsByFilter(ctx *ctx.Context, query map[string]interface{}, limit, offset int) ([]*Target, error) {
	var lst []*Target
	session := TargetFilterQueryBuild(ctx, query, limit, offset)
	err := session.Order("ident").Find(&lst).Error
	cache := make(map[int64]*BusiGroup)
	for i := 0; i < len(lst); i++ {
		lst[i].TagsJSON = strings.Fields(lst[i].Tags)
		lst[i].FillGroup(ctx, cache)
	}

	return lst, err
}

func TargetCountByFilter(ctx *ctx.Context, query map[string]interface{}) (int64, error) {
	session := TargetFilterQueryBuild(ctx, query, 0, 0)
	return Count(session)
}

func MissTargetGetsByFilter(ctx *ctx.Context, query map[string]interface{}, ts int64) ([]*Target, error) {
	var lst []*Target
	session := TargetFilterQueryBuild(ctx, query, 0, 0)
	session = session.Where("update_at < ?", ts)

	err := session.Order("ident").Find(&lst).Error
	return lst, err
}

func MissTargetCountByFilter(ctx *ctx.Context, query map[string]interface{}, ts int64) (int64, error) {
	session := TargetFilterQueryBuild(ctx, query, 0, 0)
	session = session.Where("update_at < ?", ts)
	return Count(session)
}

func TargetFilterQueryBuild(ctx *ctx.Context, query map[string]interface{}, limit, offset int) *gorm.DB {
	session := DB(ctx).Model(&Target{})
	for k, v := range query {
		session = session.Where(k, v)
	}

	if limit > 0 {
		session = session.Limit(limit).Offset(offset)
	}

	return session
}

func TargetGetsAll(ctx *ctx.Context) ([]*Target, error) {
	var lst []*Target
	err := DB(ctx).Model(&Target{}).Find(&lst).Error
	return lst, err
}

func TargetUpdateNote(ctx *ctx.Context, idents []string, note string) error {
	return DB(ctx).Model(&Target{}).Where("ident in ?", idents).Updates(map[string]interface{}{
		"note":      note,
		"update_at": time.Now().Unix(),
	}).Error
}

func TargetUpdateBgid(ctx *ctx.Context, idents []string, bgid int64, clearTags bool) error {
	fields := map[string]interface{}{
		"group_id":  bgid,
		"update_at": time.Now().Unix(),
	}

	if clearTags {
		fields["tags"] = ""
	}

	return DB(ctx).Model(&Target{}).Where("ident in ?", idents).Updates(fields).Error
}

func TargetGet(ctx *ctx.Context, where string, args ...interface{}) (*Target, error) {
	var lst []*Target
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].TagsJSON = strings.Fields(lst[0].Tags)

	return lst[0], nil
}

func TargetGetById(ctx *ctx.Context, id int64) (*Target, error) {
	return TargetGet(ctx, "id = ?", id)
}

func TargetGetByIdent(ctx *ctx.Context, ident string) (*Target, error) {
	return TargetGet(ctx, "ident = ?", ident)
}

func TargetGetTags(ctx *ctx.Context, idents []string) ([]string, error) {
	session := DB(ctx).Model(new(Target))

	var arr []string
	if len(idents) > 0 {
		session = session.Where("ident in ?", idents)
	}

	err := session.Select("distinct(tags) as tags").Pluck("tags", &arr).Error
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

func (t *Target) AddTags(ctx *ctx.Context, tags []string) error {
	for i := 0; i < len(tags); i++ {
		if !strings.Contains(t.Tags, tags[i]+" ") {
			t.Tags += tags[i] + " "
		}
	}

	arr := strings.Fields(t.Tags)
	sort.Strings(arr)

	return DB(ctx).Model(t).Updates(map[string]interface{}{
		"tags":      strings.Join(arr, " ") + " ",
		"update_at": time.Now().Unix(),
	}).Error
}

func (t *Target) DelTags(ctx *ctx.Context, tags []string) error {
	for i := 0; i < len(tags); i++ {
		t.Tags = strings.ReplaceAll(t.Tags, tags[i]+" ", "")
	}

	return DB(ctx).Model(t).Updates(map[string]interface{}{
		"tags":      t.Tags,
		"update_at": time.Now().Unix(),
	}).Error
}

func (t *Target) FillTagsMap() {
	t.TagsJSON = strings.Fields(t.Tags)
	t.TagsMap = make(map[string]string)
	for _, item := range t.TagsJSON {
		arr := strings.Split(item, "=")
		if len(arr) != 2 {
			continue
		}
		t.TagsMap[arr[0]] = arr[1]
	}
}

func (t *Target) FillMeta(meta *HostMeta) {
	t.MemUtil = meta.MemUtil
	t.CpuUtil = meta.CpuUtil
	t.CpuNum = meta.CpuNum
	t.UnixTime = meta.UnixTime
	t.Offset = meta.Offset
	t.OS = meta.OS
	t.Arch = meta.Arch
	t.RemoteAddr = meta.RemoteAddr
}

func TargetIdents(ctx *ctx.Context, ids []int64) ([]string, error) {
	var ret []string

	if len(ids) == 0 {
		return ret, nil
	}

	err := DB(ctx).Model(&Target{}).Where("id in ?", ids).Pluck("ident", &ret).Error
	return ret, err
}

func TargetIds(ctx *ctx.Context, idents []string) ([]int64, error) {
	var ret []int64

	if len(idents) == 0 {
		return ret, nil
	}

	err := DB(ctx).Model(&Target{}).Where("ident in ?", idents).Pluck("id", &ret).Error
	return ret, err
}

func IdentsFilter(ctx *ctx.Context, idents []string, where string, args ...interface{}) ([]string, error) {
	var arr []string
	if len(idents) == 0 {
		return arr, nil
	}

	err := DB(ctx).Model(&Target{}).Where("ident in ?", idents).Where(where, args...).Pluck("ident", &arr).Error
	return arr, err
}

func (m *Target) UpdateFieldsMap(ctx *ctx.Context, fields map[string]interface{}) error {
	return DB(ctx).Model(m).Updates(fields).Error
}
