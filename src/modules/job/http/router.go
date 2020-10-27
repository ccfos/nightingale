package http

import "github.com/gin-gonic/gin"

func Config(r *gin.Engine) {
	notLogin := r.Group("/api/job-ce")
	{
		notLogin.GET("/ping", ping)
		notLogin.POST("/callback", taskCallback)
		notLogin.GET("/task/:id/stdout", taskStdout)
		notLogin.GET("/task/:id/stderr", taskStderr)
		notLogin.GET("/task/:id/state", apiTaskState)
		notLogin.GET("/task/:id/result", apiTaskResult)
		notLogin.GET("/task/:id/host/:host/output", taskHostOutput)
		notLogin.GET("/task/:id/host/:host/stdout", taskHostStdout)
		notLogin.GET("/task/:id/host/:host/stderr", taskHostStderr)
		notLogin.GET("/task/:id/stdout.txt", taskStdoutTxt)
		notLogin.GET("/task/:id/stderr.txt", taskStderrTxt)
		notLogin.GET("/task/:id/stdout.json", apiTaskJSONStdouts)
		notLogin.GET("/task/:id/stderr.json", apiTaskJSONStderrs)
	}

	userLogin := r.Group("/api/job-ce").Use(shouldBeLogin())
	{
		userLogin.GET("/task-tpls", taskTplGets)
		userLogin.POST("/task-tpls", taskTplPost)
		userLogin.GET("/task-tpl/:id", taskTplGet)
		userLogin.PUT("/task-tpl/:id", taskTplPut)
		userLogin.DELETE("/task-tpl/:id", taskTplDel)
		userLogin.POST("/task-tpl/:id/run", taskTplRun)
		userLogin.PUT("/task-tpls/tags", taskTplTagsPut)
		userLogin.PUT("/task-tpls/node", taskTplNodePut)

		userLogin.POST("/tasks", taskPost)
		userLogin.GET("/tasks", taskGets)
		userLogin.GET("/task/:id", taskView)
		userLogin.PUT("/task/:id/action", taskActionPut)
		userLogin.PUT("/task/:id/host", taskHostPut)

		// 专门针对工单系统开发的接口
		userLogin.POST("/run/:id", taskRunForTT)
	}
}
