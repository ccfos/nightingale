package naming

import (
	"sort"

	"github.com/toolkits/pkg/logger"
)

func (n *Naming) IamLeader() bool {
	if !n.ctx.IsCenter {
		return false
	}

	servers, err := n.ActiveServersByEngineName()
	if err != nil {
		logger.Errorf("failed to get active servers: %v", err)
		return false
	}

	if len(servers) == 0 {
		logger.Errorf("active servers empty")
		return false
	}

	sort.Strings(servers)

	return n.heartbeatConfig.Endpoint == servers[0]
}
