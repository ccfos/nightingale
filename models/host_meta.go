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
	GlobalLabels map[string]string      `json:"global_labels"`
	ExtendInfo   map[string]interface{} `json:"extend_info"`
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
