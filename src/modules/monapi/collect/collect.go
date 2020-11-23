package collect

import (
	"fmt"
)

var collectors map[string]Collector

type Collector interface {
	Name() string
	Get(id int64) (interface{}, error)
	Gets(nids []int64) ([]interface{}, error)
	GetByNameAndNid(name string, nid int64) (interface{}, error)
	Create(data []byte, username string) error
	Update(data []byte, username string) error
	Delete(id int64, username string) error
}

func CollectorRegister(c Collector) error {
	name := c.Name()
	if _, ok := collectors[name]; ok {
		return fmt.Errorf("collector %s exists", name)
	}
	collectors[name] = c
	return nil
}

func GetCollector(name string) (Collector, error) {
	if c, ok := collectors[name]; !ok {
		return nil, fmt.Errorf("collector %s does not exist", name)
	} else {
		return c, nil
	}
}
