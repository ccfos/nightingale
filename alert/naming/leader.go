package naming

import (
	"sort"

	"github.com/toolkits/pkg/logger"
)

func (n *Naming) IamLeader() bool {
	servers, err := n.AllActiveServers()
	if err != nil {
		logger.Errorf("failed to get active servers: %v", err)
		return false
	}

	if len(servers) == 0 {
		logger.Errorf("active servers empty")
		return false
	}

	sort.Strings(servers)

	return n.Heartbeat.Endpoint == servers[0]
}
