package router

type ChartPure struct {
	Configs string `json:"configs"`
	Weight  int    `json:"weight"`
}

type ChartGroupPure struct {
	Name   string      `json:"name"`
	Weight int         `json:"weight"`
	Charts []ChartPure `json:"charts"`
}

type DashboardPure struct {
	Name        string           `json:"name"`
	Tags        string           `json:"tags"`
	Configs     string           `json:"configs"`
	ChartGroups []ChartGroupPure `json:"chart_groups"`
}
