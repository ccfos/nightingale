package stra

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/agent/config"
)

type Strategy struct {
	ID              int64                     `json:"id"`
	Name            string                    `json:"name"`        //监控策略名
	FilePath        string                    `json:"file_path"`   //文件路径
	TimeFormat      string                    `json:"time_format"` //时间格式
	Pattern         string                    `json:"pattern"`     //表达式
	Exclude         string                    `json:"-"`
	MeasurementType string                    `json:"measurement_type"`
	Interval        int64                     `json:"interval"` //采集周期
	Tags            map[string]string         `json:"tags"`
	Func            string                    `json:"func"` //采集方式（max/min/avg/cnt）
	Degree          int64                     `json:"degree"`
	Unit            string                    `json:"unit"`
	Comment         string                    `json:"comment"`
	Creator         string                    `json:"creator"`
	SrvUpdated      string                    `json:"updated"`
	LocalUpdated    int64                     `json:"-"`
	TimeReg         *regexp.Regexp            `json:"-"`
	PatternReg      *regexp.Regexp            `json:"-"`
	ExcludeReg      *regexp.Regexp            `json:"-"`
	TagRegs         map[string]*regexp.Regexp `json:"-"`
	ParseSucc       bool                      `json:"parse_succ"`
}

func GetLogCollects() []*Strategy {
	var stras []*Strategy
	if config.Config.Stra.Enable {
		strasMap := Collect.GetLogConfig()

		for _, s := range strasMap {
			stra := ToStrategy(s)
			stras = append(stras, stra)
		}
	}

	//从文件中读取
	stras = append(stras, GetCollectFromFile(config.Config.Stra.LogPath)...)

	parsePattern(stras)
	updateRegs(stras)

	return stras
}

func GetCollectFromFile(logPath string) []*Strategy {
	logger.Info("get collects from local file")
	var stras []*Strategy

	files, err := file.FilesUnder(logPath)
	if err != nil {
		logger.Error(err)
		return stras
	}

	//扫描文件采集配置
	for _, f := range files {
		err := checkName(f)
		if err != nil {
			logger.Warningf("read file name err:%s %v", f, err)
			continue
		}

		stra := Strategy{}

		b, err := file.ToBytes(logPath + "/" + f)
		if err != nil {
			logger.Warningf("read file name err:%s %v", f, err)
			continue
		}

		err = json.Unmarshal(b, &stra)
		if err != nil {
			logger.Warningf("read file name err:%s %v", f, err)
			continue
		}

		//todo 配置校验

		stra.ID = genStraID(stra.Name, string(b))
		stras = append(stras, &stra)
	}

	return stras
}

func genStraID(name, body string) int64 {
	var id int64
	all := name + body
	if len(all) < 1 {
		return id
	}

	id = int64(all[0])

	for i := 1; i < len(all); i++ {
		id += int64(all[i])
		id += int64(all[i] - all[i-1])
	}

	id += 1000000 //避免与web端配置的id冲突
	return id
}

func ToStrategy(p *models.LogCollect) *Strategy {
	s := Strategy{}
	s.ID = p.Id
	s.Name = p.Name
	s.FilePath = p.FilePath
	s.TimeFormat = p.TimeFormat
	s.Pattern = p.Pattern
	s.MeasurementType = p.CollectType
	s.Interval = int64(p.Step)
	s.Tags = DeepCopyStringMap(p.Tags)
	s.Func = p.Func
	s.Degree = int64(p.Degree)
	s.Unit = p.Unit
	s.Comment = p.Comment
	s.Creator = p.Creator
	s.SrvUpdated = p.LastUpdated.String()
	s.LocalUpdated = p.LocalUpdated

	return &s
}

func DeepCopyStringMap(p map[string]string) map[string]string {
	r := make(map[string]string, len(p))
	for k, v := range p {
		r[k] = v
	}
	return r
}

const PATTERN_EXCLUDE_PARTITION = "```EXCLUDE```"

func parsePattern(strategies []*Strategy) {
	for _, st := range strategies {
		patList := strings.Split(st.Pattern, PATTERN_EXCLUDE_PARTITION)

		if len(patList) == 1 {
			st.Pattern = strings.TrimSpace(st.Pattern)
			continue
		} else if len(patList) >= 2 {
			st.Pattern = strings.TrimSpace(patList[0])
			st.Exclude = strings.TrimSpace(patList[1])
			continue
		} else {
			logger.Errorf("bad pattern to parse : [%s]", st.Pattern)
		}
	}
}

func updateRegs(strategies []*Strategy) {
	for _, st := range strategies {
		st.TagRegs = make(map[string]*regexp.Regexp)
		st.ParseSucc = false

		//更新时间正则
		pat, _ := GetPatAndTimeFormat(st.TimeFormat)
		reg, err := regexp.Compile(pat)
		if err != nil {
			logger.Errorf("compile time regexp failed:[sid:%d][format:%s][pat:%s][err:%v]", st.ID, st.TimeFormat, pat, err)
			continue
		}
		st.TimeReg = reg //拿到时间正则

		if len(st.Pattern) == 0 && len(st.Exclude) == 0 {
			logger.Errorf("pattern and exclude are all empty, sid:[%d]", st.ID)
			continue
		}

		//更新pattern
		if len(st.Pattern) != 0 {
			reg, err = regexp.Compile(st.Pattern)
			if err != nil {
				logger.Errorf("compile pattern regexp failed:[sid:%d][pat:%s][err:%v]", st.ID, st.Pattern, err)
				continue
			}
			st.PatternReg = reg
		}

		//更新exclude
		if len(st.Exclude) != 0 {
			reg, err = regexp.Compile(st.Exclude)
			if err != nil {
				logger.Errorf("compile exclude regexp failed:[sid:%d][pat:%s][err:%v]", st.ID, st.Exclude, err)
				continue
			}
			st.ExcludeReg = reg
		}

		//更新tags
		for tagk, tagv := range st.Tags {
			reg, err = regexp.Compile(tagv)
			if err != nil {
				logger.Errorf("compile tag failed:[sid:%d][pat:%s][err:%v]", st.ID, st.Exclude, err)
				continue
			}
			st.TagRegs[tagk] = reg
		}
		st.ParseSucc = true
	}
}

func checkName(f string) (err error) {
	if !strings.Contains(f, "log.") {
		err = fmt.Errorf("name illege not contain log. %s", f)
		return
	}

	arr := strings.Split(f, ".")
	if len(arr) < 3 {
		err = fmt.Errorf("name illege %s len:%d len < 3 ", f, len(arr))
		return
	}

	if arr[len(arr)-1] != "json" {
		err = fmt.Errorf("name illege %s not json file", f)
		return
	}

	return
}

func GetPatAndTimeFormat(tf string) (string, string) {
	var pat, timeFormat string
	switch tf {
	case "dd/mmm/yyyy:HH:MM:SS":
		pat = `([012][0-9]|3[01])/[JFMASONDjfmasond][a-zA-Z]{2}/(2[0-9]{3}):([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02/Jan/2006:15:04:05"
	case "dd/mmm/yyyy HH:MM:SS":
		pat = `([012][0-9]|3[01])/[JFMASONDjfmasond][a-zA-Z]{2}/(2[0-9]{3})\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02/Jan/2006 15:04:05"
	case "yyyy-mm-ddTHH:MM:SS":
		pat = `(2[0-9]{3})-(0[1-9]|1[012])-([012][0-9]|3[01])T([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "2006-01-02T15:04:05"
	case "dd-mmm-yyyy HH:MM:SS":
		pat = `([012][0-9]|3[01])-[JFMASONDjfmasond][a-zA-Z]{2}-(2[0-9]{3})\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02-Jan-2006 15:04:05"
	case "yyyy-mm-dd HH:MM:SS":
		pat = `(2[0-9]{3})-(0[1-9]|1[012])-([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "2006-01-02 15:04:05"
	case "yyyy/mm/dd HH:MM:SS":
		pat = `(2[0-9]{3})/(0[1-9]|1[012])/([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "2006/01/02 15:04:05"
	case "yyyymmdd HH:MM:SS":
		pat = `(2[0-9]{3})(0[1-9]|1[012])([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "20060102 15:04:05"
	case "mmm dd HH:MM:SS":
		pat = `[JFMASONDjfmasond][a-zA-Z]{2}\s+([1-9]|[1-2][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "Jan 2 15:04:05"
	case "mmdd HH:MM:SS":
		pat = `(0[1-9]|1[012])([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "0102 15:04:05"
	case "dd/mm/yyyy:HH:MM:SS":
		pat = `([012][0-9]|3[01])/(0[1-9]|1[012])/(2[0-9]{3}):([01][0-9]|2[0-4])(:[012345][0-9]){2}`
		timeFormat = "02/01/2006:15:04:05"
        case "mm-dd HH:MM:SS":
                pat = `(0[1-9]|1[012])-([012][0-9]|3[01])\s([01][0-9]|2[0-4])(:[012345][0-9]){2}`
                timeFormat = "01-02 15:04:05"
	default:
		logger.Errorf("match time pac failed : [timeFormat:%s]", tf)
		return "", ""
	}
	return pat, timeFormat
}
