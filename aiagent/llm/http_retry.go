package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// doHTTPWithRetry 发起一次带重试的 HTTP 请求，读完整响应体后关闭。
// newReq 负责在每次重试时构造一个全新的 *http.Request（body 会在 Client.Do 中被消费，
// 不能重用）。setHeaders 安装 provider 特有的请求头。apiName 用于错误信息，
// 例如 "OpenAI" / "Claude" / "Gemini"。
//
// 非流式路径用这个函数；流式路径用 doHTTPStreamWithRetry，保持 resp.Body 打开由调用方读。
func doHTTPWithRetry(
	ctx context.Context,
	client *http.Client,
	apiName string,
	newReq func() (*http.Request, error),
	setHeaders func(*http.Request),
) ([]byte, error) {
	var lastErr error
	retryWait := initialRetryWait

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryWait):
			}
			retryWait *= 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		httpReq, err := newReq()
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		setHeaders(httpReq)

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("failed to read response: %w", readErr)
			continue
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("%s API error (status %d): %s", apiName, resp.StatusCode, string(body))
			if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
				continue
			}
			return nil, lastErr
		}
		return body, nil
	}
	return nil, lastErr
}

// doHTTPStreamWithRetry 面向流式响应：成功时返回 *http.Response 让调用方自行读取 Body，
// 失败时保证 Body 已关闭。语义与 doHTTPWithRetry 相同，只是成功路径不读 Body。
func doHTTPStreamWithRetry(
	ctx context.Context,
	client *http.Client,
	apiName string,
	newReq func() (*http.Request, error),
	setHeaders func(*http.Request),
) (*http.Response, error) {
	var lastErr error
	retryWait := initialRetryWait

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryWait):
			}
			retryWait *= 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		httpReq, err := newReq()
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		setHeaders(httpReq)

		resp, err := client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			continue
		}

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("%s API error (status %d): %s", apiName, resp.StatusCode, string(body))
			if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
				continue
			}
			return nil, lastErr
		}
		return resp, nil
	}
	return nil, lastErr
}

// isRetryableStatus 判断 HTTP 状态码是否值得重试（rate limit + 5xx）
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}
