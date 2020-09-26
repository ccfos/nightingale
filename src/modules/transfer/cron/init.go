package cron

func Init() {
	go GetStrategy()
	go RebuildJudgePool()
	go UpdateJudgeQueue()
	go GetAggrCalcStrategy()
}
