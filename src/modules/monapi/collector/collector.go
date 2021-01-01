package collector

import (
	"errors"
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
)

var (
	collectors       = map[string]Collector{}
	remoteCollectors = []string{}
	localCollectors  = []string{}
	errUnsupported   = errors.New("unsupported")
)

type Category string

const (
	RemoteCategory Category = "remote" // used for prober
	LocalCategory  Category = "local"  // used for agent
)

type Collector interface {
	Name() string
	Category() Category
	Get(id int64) (interface{}, error)
	Gets(nids []int64) ([]interface{}, error)
	GetByNameAndNid(name string, nid int64) (interface{}, error)
	Create(data []byte, username string) error
	Update(data []byte, username string) error
	Delete(id int64, username string) error
	Template() (interface{}, error)
	TelegrafInput(*models.CollectRule) (telegraf.Input, error)
}

func CollectorRegister(c Collector) error {
	name := c.Name()
	if _, ok := collectors[name]; ok {
		return fmt.Errorf("collector %s exists", name)
	}
	collectors[name] = c

	if c.Category() == RemoteCategory {
		remoteCollectors = append(remoteCollectors, name)
	}

	if c.Category() == LocalCategory {
		localCollectors = append(localCollectors, name)
	}

	return nil
}

func GetCollector(name string) (Collector, error) {
	if c, ok := collectors[name]; !ok {
		return nil, fmt.Errorf("collector %s does not exist", name)
	} else {
		return c, nil
	}
}

func GetRemoteCollectors() []string {
	return remoteCollectors
}

func GetLocalCollectors() []string {
	return localCollectors
}

func _s(format string, a ...interface{}) string {
	logger.Debugf(`    "%s": "%s",`, format, format)
	return i18n.Sprintf(format, a...)
}
