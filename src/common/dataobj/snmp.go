package dataobj

import (
	"regexp"
	"time"
)

const (
	COMMON_MODULE = "common"
)

type IPAndSnmp struct {
	IP          string    `json:"ip"`
	Module      string    `json:"module"`
	Version     string    `json:"version"`
	Auth        string    `json:"auth"`
	Region      string    `json:"region"`
	Step        int       `json:"step"`
	Timeout     int       `json:"timeout"`
	Port        int       `json:"port"`
	Metric      Metric    `json:"metric"`
	LastUpdated time.Time `json:"last_updated"`
}

type Metric struct {
	Name           string                     `yaml:"name" json:"name"`
	Oid            string                     `yaml:"oid" json:"oid"`
	Type           string                     `yaml:"type" json:"type"`
	Help           string                     `yaml:"help" json:"help"`
	Indexes        []*Index                   `yaml:"indexes" json:"indexes,omitempty"`
	Lookups        []*Lookup                  `yaml:"lookups" json:"lookups,omitempty"`
	RegexpExtracts map[string][]RegexpExtract `yaml:"regex_extracts" json:"regex_extracts,omitempty"`
	EnumValues     map[int]string             `yaml:"enum_values" json:"enum_values,omitempty"`
}

type Index struct {
	Labelname string `yaml:"labelname" json:"labelname"`
	Type      string `yaml:"type" json:"type"`
	FixedSize int    `yaml:"fixed_size" json:"fixed_size,omitempty"`
	Implied   bool   `yaml:"implied" json:"implied,omitempty"`
}

type Lookup struct {
	Labels    []string `yaml:"labels" json:"labels"`
	Labelname string   `yaml:"labelname" json:"labelname"`
	Oid       string   `yaml:"oid" json:"oid,omitempty"`
	Type      string   `yaml:"type" json:"type,omitempty"`
}

type RegexpExtract struct {
	Value string `yaml:"value" json:"value"`
	Regex Regexp `yaml:"regex" json:"regex"`
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshalable.
type Regexp struct {
	*regexp.Regexp
}

type IfTags struct {
	IfName  string
	IfIndex string
}

// Secret is a string that must not be revealed on marshaling.
type Secret string

type Auth struct {
	Community     Secret `json:"community,omitempty"`
	SecurityLevel string `json:"security_level,omitempty"`
	Username      string `json:"username,omitempty"`
	Password      Secret `json:"password,omitempty"`
	AuthProtocol  string `json:"auth_protocol,omitempty"`
	PrivProtocol  string `json:"priv_protocol,omitempty"`
	PrivPassword  Secret `json:"priv_password,omitempty"`
	ContextName   string `json:"context_name,omitempty"`
}
