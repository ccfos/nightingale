package config

const Version = 1

const JudgesReplicas = 500
const DetectorReplicas = 500

const (
	RECOVERY = "recovery"
	ALERT    = "alert"
)

var (
	EventTypeMap = map[string]string{RECOVERY: "恢复", ALERT: "报警"}
)
