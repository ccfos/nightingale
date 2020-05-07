package dataobj

import (
	"strconv"

	"github.com/didi/nightingale/src/toolkits/str"

	gstr "github.com/toolkits/pkg/str"
)

type JudgeItem struct {
	Endpoint  string            `json:"endpoint"`
	Metric    string            `json:"metric"`
	Tags      string            `json:"tags"`
	TagsMap   map[string]string `json:"tagsMap"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	DsType    string            `json:"dstype"`
	Step      int               `json:"step"`
	Sid       int64             `json:"sid"`
	Extra     string            `json:"extra"`
}

func (j *JudgeItem) PrimaryKey() string {
	return str.PK(j.Endpoint, j.Metric, j.Tags)
}

func (j *JudgeItem) MD5() string {
	return gstr.MD5(str.PK(strconv.FormatInt(j.Sid, 16), j.Endpoint, j.Metric, str.SortedTags(j.TagsMap)))
}
