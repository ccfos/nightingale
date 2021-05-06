package cron

func Init() {
	go GetStrategy()
	go RebuildJudgePool()
	go UpdateJudgeQueue()

	//monapi
	go CheckJudgeNodes()
	go SyncStras()
	go CleanStraLoop()
	go SyncCollects()
	go CleanCollectLoop()

	//rdb
	go ConsumeMail()
	go ConsumeSms()
	go ConsumeVoice()
	go ConsumeIm()
	go CleanerLoop()
}
