package dataobj

type Message struct {
	Tos     []string `json:"tos"`
	Subject string   `json:"subject"`
	Content string   `json:"content"`
}
