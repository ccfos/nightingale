package router

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// 客户端把数据推给 pushgw，pushgw 再转发给 prometheus。
// 这个方法中，pushgw 不做任何处理，不解析 http request body，直接转发给配置文件中指定的多个 writers。
// 相比 /prometheus/v1/write 方法，这个方法不需要在内存里搞很多队列，性能更好。
// 注意：后来想了想这个方法也不太合适，不推荐用户使用。还是应该继续优化一下 /prometheus/v1/write 方法的队列逻辑。
func (rt *Router) proxyRemoteWrite(c *gin.Context) {
	// 读取 request body
	bs, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// 拿到所有的 writer 配置
	for index := range rt.Pushgw.Writers {
		writer := rt.Pushgw.Writers[index]

		targetUrl := writer.Url
		if c.Request.URL.RawQuery != "" {
			// 如果有 querystring，把 querystring 拼接到 url 后面
			if strings.Contains(writer.Url, "?") {
				targetUrl += "&" + c.Request.URL.RawQuery
			} else {
				targetUrl += "?" + c.Request.URL.RawQuery
			}
		}

		// 把 bs 放到 http request 中发给 writer 中的 HTTPTransport
		req, err := http.NewRequest("POST", targetUrl, bytes.NewReader(bs))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

		// 把 header 转发给后端
		contentType := c.GetHeader("Content-Type")
		if contentType == "" {
			contentType = "application/x-protobuf"
		}
		req.Header.Set("Content-Type", contentType)

		contentEncoding := c.GetHeader("Content-Encoding")
		if contentEncoding == "" {
			contentEncoding = "snappy"
		}
		req.Header.Set("Content-Encoding", contentEncoding)

		userAgent := c.GetHeader("User-Agent")
		if userAgent == "" {
			userAgent = "n9e"
		} else {
			userAgent += "-n9e"
		}
		req.Header.Set("User-Agent", userAgent)

		rwVersion := c.GetHeader("X-Prometheus-Remote-Write-Version")
		if rwVersion == "" {
			rwVersion = "0.1.0"
		}
		req.Header.Set("X-Prometheus-Remote-Write-Version", rwVersion)

		if writer.BasicAuthUser != "" {
			req.SetBasicAuth(writer.BasicAuthUser, writer.BasicAuthPass)
		}

		headerCount := len(writer.Headers)
		if headerCount > 0 && headerCount%2 == 0 {
			for i := 0; i < len(writer.Headers); i += 2 {
				req.Header.Add(writer.Headers[i], writer.Headers[i+1])
				if writer.Headers[i] == "Host" {
					req.Host = writer.Headers[i+1]
				}
			}
		}

		client := http.Client{
			Timeout:   time.Duration(writer.Timeout) * time.Millisecond,
			Transport: writer.HTTPTransport,
		}

		res, err := client.Do(req)
		if err != nil {
			logger.Warningf("[forward-timeseries] failed to do request. url=%s error=%v", targetUrl, err)
			continue
		}

		defer res.Body.Close()

		if res.StatusCode >= 400 {
			body, err := io.ReadAll(res.Body)
			if err != nil {
				logger.Warningf("[forward-timeseries] failed to read response body. url=%s error=%v", targetUrl, err)
				continue
			}

			logger.Warningf("[forward-timeseries] response status code ge 400. url=%s status_code=%d response=%s", targetUrl, res.StatusCode, string(body))
			continue
		}

	}

}
