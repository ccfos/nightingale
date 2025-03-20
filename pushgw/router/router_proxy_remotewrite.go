package router

import "github.com/gin-gonic/gin"

// 客户端把数据推给 pushgw，pushgw 再转发给 prometheus。
// 这个方法中，pushgw 不做任何处理，不解析 http request body，直接转发给配置文件中指定的多个 writers。
// 相比 /prometheus/v1/write 方法，这个方法不需要在内存里搞很多队列，性能更好。
func (rt *Router) proxyRemoteWrite(c *gin.Context) {

}
