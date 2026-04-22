package mcp

const (
	// MCP 传输类型
	MCPTransportStdio = "stdio" // 标准输入/输出传输
	MCPTransportSSE   = "sse"   // HTTP Server-Sent Events 传输

	// 默认超时
	DefaultMCPTimeout        = 30000 // 30 秒
	DefaultMCPConnectTimeout = 10000 // 10 秒
)

// Config MCP 服务器配置（在 AIAgentConfig 中使用）
type Config struct {
	// MCP 服务器列表
	Servers []ServerConfig `json:"servers"`
}

// ServerConfig 单个 MCP 服务器配置
type ServerConfig struct {
	// 服务器名称（唯一标识）
	Name string `json:"name"`

	// 传输类型：stdio 或 sse
	Transport string `json:"transport"`

	// === stdio 传输配置 ===
	Command string            `json:"command,omitempty"` // 启动命令
	Args    []string          `json:"args,omitempty"`    // 命令参数
	Env     map[string]string `json:"env,omitempty"`     // 环境变量（支持 ${VAR} 从系统环境变量读取）

	// === SSE 传输配置 ===
	URL           string            `json:"url,omitempty"`             // SSE 服务器 URL
	Headers       map[string]string `json:"headers,omitempty"`         // 请求头（支持 ${VAR} 从系统环境变量读取）
	SkipSSLVerify bool              `json:"skip_ssl_verify,omitempty"` // 跳过 SSL 验证

	// === 鉴权配置（SSE 传输）===
	// 便捷鉴权配置，会自动设置对应的 Header
	AuthType string `json:"auth_type,omitempty"` // 鉴权类型：bearer, api_key, basic
	APIKey   string `json:"api_key,omitempty"`   // API Key（支持 ${VAR} 从系统环境变量读取）
	Username string `json:"username,omitempty"`  // Basic Auth 用户名
	Password string `json:"password,omitempty"`  // Basic Auth 密码（支持 ${VAR}）

	// 通用配置
	Timeout        int `json:"timeout,omitempty"`         // 工具调用超时（毫秒）
	ConnectTimeout int `json:"connect_timeout,omitempty"` // 连接超时（毫秒）
}

// ToolConfig MCP 工具配置（在 AgentTool 中使用）
type ToolConfig struct {
	// MCP 服务器名称（引用 Config.Servers 中的配置）
	ServerName string `json:"server_name"`

	// 工具名称（MCP 服务器返回的工具名）
	ToolName string `json:"tool_name"`
}

// Tool MCP 工具定义（用于内部表示）
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ToolsCallResult 工具调用结果
type ToolsCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content 工具返回内容
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}
