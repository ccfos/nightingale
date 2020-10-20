package dataobj

type Notify struct {
	Tos     []string `json:"tos"`
	Subject string   `json:"subject,omitempty"`
	Content string   `json:"content"`
}
