package models

import "encoding/json"

type HostMeta struct {
	AgentVersion string  `json:"agent_version"`
	OS           string  `json:"os"`
	Arch         string  `json:"arch"`
	Hostname     string  `json:"hostname"`
	CpuNum       int     `json:"cpu_num"`
	CpuUtil      float64 `json:"cpu_util"`
	MemUtil      float64 `json:"mem_util"`
	Offset       int64   `json:"offset"`
	UnixTime     int64   `json:"unixtime"`
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
