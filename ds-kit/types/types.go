package types

const (
	LogExtractValueTypeLong   = "long"
	LogExtractValueTypeFloat  = "float"
	LogExtractValueTypeText   = "text"
	LogExtractValueTypeDate   = "date"
	LogExtractValueTypeBool   = "bool"
	LogExtractValueTypeObject = "object"
	LogExtractValueTypeArray  = "array"
	LogExtractValueTypeJSON   = "json"
)

type ColumnProperty struct {
	Field     string `json:"field"`
	Type      string `json:"type"`
	Type2     string `json:"type2,omitempty"` // field_property.Type
	Indexable bool   `json:"indexable"`       // 是否可以索引
}
