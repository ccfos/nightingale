package mcp

// jsonRPCResponse JSON-RPC 响应（HTTP 探测使用）
type jsonRPCResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result"`
	Error   *jsonRPCError          `json:"error"`
}

// jsonRPCError JSON-RPC 错误
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
