package dataobj

type TaskMetaResponse struct {
	Message string
	Script  string
	Args    string
	Account string
}

type ReportTask struct {
	Id     int64
	Clock  int64
	Status string
	Stdout string
	Stderr string
}

type ReportRequest struct {
	Ident       string
	ReportTasks []ReportTask
}

type AssignTask struct {
	Id     int64
	Clock  int64
	Action string
}

type ReportResponse struct {
	Message     string
	AssignTasks []AssignTask
}
