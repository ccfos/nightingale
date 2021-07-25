package timer

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

func SyncCollectRules() {
	err := syncCollectRules()
	if err != nil {
		fmt.Println("timer: sync collect rules fail:", err)
		exit(1)
	}

	go loopSyncCollectRules()
}

func loopSyncCollectRules() {
	randtime := rand.Intn(10000)
	fmt.Printf("timer: sync collect rules: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(60) * time.Second

	for {
		time.Sleep(interval)
		err := syncCollectRules()
		if err != nil {
			logger.Warning("timer: sync collect rules fail:", err)
		}
	}
}

func syncCollectRules() error {
	start := time.Now()

	collectRules, err := models.CollectRuleGetAll()
	if err != nil {
		return err
	}

	// ident -> collect_rule1, collect_rule2 ...
	collectRulesMap := make(map[string][]*models.CollectRule)
	// classpath prefix -> classpaths
	prefixClasspath := make(map[string][]models.Classpath)

	for i := range collectRules {
		classpathAndRes, exists := cache.ClasspathRes.Get(collectRules[i].ClasspathId)
		if !exists {
			continue
		}

		err := changeCollectRule(collectRules[i])
		if err != nil {
			logger.Errorf("change collect:%+v err:%v", collectRules[i], err)
			continue
		}

		if collectRules[i].PrefixMatch == 0 {
			// 我这个采集规则所关联的节点下面直接挂载的那些资源，都关联本采集规则
			for _, ident := range classpathAndRes.Res {
				if _, exists := collectRulesMap[ident]; !exists {
					collectRulesMap[ident] = []*models.CollectRule{collectRules[i]}
				} else {
					collectRulesMap[ident] = append(collectRulesMap[ident], collectRules[i])
				}
			}
		} else {
			// 我这个采集规则关联的节点下面的所有的子节点，这个计算量有点大，可能是个问题
			cps, exists := prefixClasspath[classpathAndRes.Classpath.Path]
			if !exists {
				cps, err = models.ClasspathGetsByPrefix(classpathAndRes.Classpath.Path)
				if err != nil {
					logger.Errorf("collectRule %+v get classpath err:%v", collectRules[i], err)
					continue
				}
				prefixClasspath[classpathAndRes.Classpath.Path] = cps
			}

			for j := range cps {
				classpathAndRes, exists := cache.ClasspathRes.Get(cps[j].Id)
				if !exists {
					continue
				}

				for _, ident := range classpathAndRes.Res {
					if _, exists := collectRulesMap[ident]; !exists {
						collectRulesMap[ident] = []*models.CollectRule{collectRules[i]}
					} else {
						collectRulesMap[ident] = append(collectRulesMap[ident], collectRules[i])
					}
				}
			}
		}

	}

	cache.CollectRulesOfIdent.SetAll(collectRulesMap)
	logger.Debugf("timer: sync collect rules done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}

// 将服务端collect rule转换为agent需要的格式
func changeCollectRule(rule *models.CollectRule) error {
	switch rule.Type {
	case "port":
		var conf models.PortConfig
		err := json.Unmarshal([]byte(rule.Data), &conf)
		if err != nil {
			return err
		}

		config := PortCollectFormat{
			Instances: []struct {
				MinCollectionInterval int      `json:"min_collection_interval,omitempty"`
				Tags                  []string `json:"tags,omitempty"`
				Protocol              string   `json:"protocol" description:"udp or tcp"`
				Port                  int      `json:"port"`
				Timeout               int      `json:"timeout"`
			}{{
				MinCollectionInterval: rule.Step,
				Tags:                  strings.Fields(strings.Replace(rule.AppendTags, "=", ":", 1)),
				Protocol:              conf.Protocol,
				Port:                  conf.Port,
				Timeout:               conf.Timeout,
			}},
		}

		data, err := json.Marshal(config)
		if err != nil {
			return err
		}
		rule.Data = string(data)

	case "script":
		var conf models.ScriptConfig
		err := json.Unmarshal([]byte(rule.Data), &conf)
		if err != nil {
			return err
		}

		config := ScriptCollectFormat{
			Instances: []struct {
				MinCollectionInterval int               `json:"min_collection_interval,omitempty"`
				FilePath              string            `json:"file_path"`
				Root                  string            `json:"root"`
				Params                string            `json:"params"`
				Env                   map[string]string `json:"env"`
				Stdin                 string            `json:"stdin"`
				Timeout               int               `json:"timeout"`
			}{{
				MinCollectionInterval: rule.Step,
				FilePath:              conf.Path,
				Params:                conf.Params,
				Env:                   conf.Env,
				Stdin:                 conf.Stdin,
				Timeout:               conf.Timeout,
			}},
		}

		data, err := json.Marshal(config)
		if err != nil {
			return err
		}
		rule.Data = string(data)
	case "log":
		var conf models.LogConfig
		err := json.Unmarshal([]byte(rule.Data), &conf)
		if err != nil {
			return err
		}

		config := LogCollectFormat{
			Instances: []struct {
				MetricName  string            `json:"metric_name"` //
				FilePath    string            `json:"file_path"`
				Pattern     string            `json:"pattern"`
				TagsPattern map[string]string `json:"tags_pattern"`
				Func        string            `json:"func"`
			}{{
				MetricName:  rule.Name,
				FilePath:    conf.FilePath,
				Pattern:     conf.Pattern,
				TagsPattern: conf.TagsPattern,
				Func:        conf.Func,
			}},
		}

		data, err := json.Marshal(config)
		if err != nil {
			return err
		}
		rule.Data = string(data)
	case "process":
		var conf models.ProcConfig
		err := json.Unmarshal([]byte(rule.Data), &conf)
		if err != nil {
			return err
		}

		config := ProcCollectFormat{
			Instances: []struct {
				MinCollectionInterval int      `json:"min_collection_interval,omitempty"`
				Tags                  []string `json:"tags,omitempty"`
				Target                string   `json:"target"`
				CollectMethod         string   `json:"collect_method" description:"name or cmdline"`
			}{{
				MinCollectionInterval: rule.Step,
				Tags:                  strings.Fields(strings.Replace(rule.AppendTags, "=", ":", 1)),
				Target:                conf.Param,
				CollectMethod:         conf.Method,
			}},
		}

		data, err := json.Marshal(config)
		if err != nil {
			return err
		}
		rule.Data = string(data)
	}

	return nil
}

type ScriptCollectFormat struct {
	Instances []struct {
		MinCollectionInterval int               `json:"min_collection_interval,omitempty"`
		FilePath              string            `json:"file_path"`
		Root                  string            `json:"root"`
		Params                string            `json:"params"`
		Env                   map[string]string `json:"env"`
		Stdin                 string            `json:"stdin"`
		Timeout               int               `json:"timeout"`
	} `json:"instances"`
}

type PortCollectFormat struct {
	Instances []struct {
		MinCollectionInterval int      `json:"min_collection_interval,omitempty"`
		Tags                  []string `json:"tags,omitempty"`
		Protocol              string   `json:"protocol" description:"udp or tcp"`
		Port                  int      `json:"port"`
		Timeout               int      `json:"timeout"`
	} `json:"instances"`
}

type LogCollectFormat struct {
	Instances []struct {
		MetricName  string            `json:"metric_name"`  //
		FilePath    string            `json:"file_path"`    //
		Pattern     string            `json:"pattern"`      //
		TagsPattern map[string]string `json:"tags_pattern"` //
		Func        string            `json:"func"`         // count(c), histogram(h)
	} `json:"instances"`
}

type ProcCollectFormat struct {
	Instances []struct {
		MinCollectionInterval int      `json:"min_collection_interval,omitempty"`
		Tags                  []string `json:"tags,omitempty"`
		Target                string   `json:"target"`
		CollectMethod         string   `json:"collect_method" description:"name or cmdline"`
	} `json:"instances"`
}
