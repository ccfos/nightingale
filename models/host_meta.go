package models

import "encoding/json"

type HostMeta struct {
	AgentVersion string                 `json:"agent_version"`
	OS           string                 `json:"os"`
	Arch         string                 `json:"arch"`
	Hostname     string                 `json:"hostname"`
	CpuNum       int                    `json:"cpu_num"`
	CpuUtil      float64                `json:"cpu_util"`
	MemUtil      float64                `json:"mem_util"`
	Offset       int64                  `json:"offset"`
	UnixTime     int64                  `json:"unixtime"`
	RemoteAddr   string                 `json:"remote_addr"`
	HostIp       string                 `json:"host_ip"`
	EngineName   string                 `json:"engine_name"`
	GlobalLabels map[string]string      `json:"global_labels"`
	ExtendInfo   map[string]interface{} `json:"extend_info"`
	Config       interface{}            `json:"config"`
	// LocalIbexEnable 采集器上报的「本地配置文件是否启用了 ibex（命令通道）」事实。
	// 指针：老采集器不上报为 nil（区分「未上报」与「上报 false」），服务端据此派生 ibex_status。
	LocalIbexEnable *bool `json:"local_ibex_enable,omitempty"`
}

type HostUpdateTime struct {
	Ident      string `json:"ident"`
	UpdateTime int64  `json:"update_time"`
}

func (h HostUpdateTime) MarshalBinary() ([]byte, error) {
	return json.Marshal(h)
}

func (h *HostUpdateTime) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, h)
}

type HostUnixTime struct {
	Ident    string `json:"ident"`
	UnixTime int64  `json:"unixtime"`
}

func (h HostMeta) MarshalBinary() ([]byte, error) {
	return json.Marshal(h)
}

func (h *HostMeta) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, h)
}

func WrapIdent(ident string) string {
	return "n9e_meta_" + ident
}

func WrapExtendIdent(ident string) string {
	return "n9e_extend_meta_" + ident
}

func WrapIdentUpdateTime(ident string) string {
	return "n9e_meta_update_time_" + ident
}
