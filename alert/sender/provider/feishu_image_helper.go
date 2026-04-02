package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// 获取 feishu/lark 开放平台 tenant access token
func getOpenPlatformTenantAccessToken(ctx context.Context, client *http.Client, appID, appSecret, tokenURL string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	body, _ := json.Marshal(map[string]string{"app_id": appID, "app_secret": appSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var out struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return "", fmt.Errorf("parse token response failed: %w, body: %s", err, string(bs))
	}
	if out.Code != 0 || out.TenantAccessToken == "" {
		return "", fmt.Errorf("get token failed: code=%d msg=%s", out.Code, out.Msg)
	}
	return out.TenantAccessToken, nil
}

// 上传 feishu/lark 开放平台图片
func uploadOpenPlatformImage(ctx context.Context, client *http.Client, token, imageBase64, imageURL string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if token == "" {
		return "", errors.New("tenant access token cannot be empty")
	}
	imgBytes, err := decodeBase64Payload(imageBase64)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err = writer.WriteField("image_type", "message"); err != nil {
		return "", err
	}
	part, err := writer.CreateFormFile("image", "image.jpg")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(imgBytes); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, imageURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ImageKey string `json:"image_key"`
		} `json:"data"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return "", fmt.Errorf("parse upload image response failed: %w, body: %s", err, string(bs))
	}
	if out.Code != 0 || out.Data.ImageKey == "" {
		return "", fmt.Errorf("upload image failed: code=%d msg=%s", out.Code, out.Msg)
	}
	return out.Data.ImageKey, nil
}
