package collector

import (
	"errors"
	"fmt"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
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

// Collector is an abstract, pluggable interface for monapi & prober.
type Collector interface {
	// Name return the collector name
	Name() string
	// Category return the collector category,  remote | local
	Category() Category
	// Get return a collectRule by collectRule.Id
	Get(id int64) (interface{}, error)
	// Gets return collectRule list by node ids
	Gets(nids []int64) ([]interface{}, error)
	// GetByNameAndNid return collectRule by collectRule.Name & collectRule.Nid
	GetByNameAndNid(name string, nid int64) (interface{}, error)
	// Create a collectRule by []byte format, witch could be able to unmarshal with a collectRule struct
	Create(data []byte, username string) error
	// Update a collectRule by []byte format, witch could be able to unmarshal with a collectRule struct
	Update(data []byte, username string) error
	// Delete a collectRule by collectRule.Id with operator's name
	Delete(id int64, username string) error
	// Template return a template used for UI render
	Template() (interface{}, error)
	// TelegrafInput return a telegraf.Input interface, this is called by prober.manager every collectRule.Step
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
	return i18n.Sprintf(format, a...)
}
