package mcp

import (
	"context"
	"sync"

	"github.com/toolkits/pkg/logger"
)

// ClientManager MCP 客户端管理器
type ClientManager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewClientManager 创建 MCP 客户端管理器
func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[string]*Client),
	}
}

// GetOrCreateClient 获取或创建 MCP 客户端
func (m *ClientManager) GetOrCreateClient(ctx context.Context, config *ServerConfig) (*Client, error) {
	m.mu.RLock()
	client, ok := m.clients[config.Name]
	m.mu.RUnlock()

	if ok {
		return client, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 再次检查（double-check locking）
	if client, ok := m.clients[config.Name]; ok {
		return client, nil
	}

	// 创建新客户端
	client, err := NewClient(config)
	if err != nil {
		return nil, err
	}

	// 连接
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}

	m.clients[config.Name] = client
	return client, nil
}

// CloseAll 关闭所有客户端
func (m *ClientManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			logger.Warningf("Failed to close MCP client %s: %v", name, err)
		}
	}
	m.clients = make(map[string]*Client)
}
