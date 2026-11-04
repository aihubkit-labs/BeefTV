// Package aihubkit contains the optional account and storage integration. It
// deliberately does not replace the official provider/plugin implementations.
package aihubkit

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"time"

	"infinite-canvas/backend/internal/integrations/aihubkit/branding"
	"infinite-canvas/backend/internal/outbound"
)

const Origin = "https://apimodel.aihubkit.com"
const credentialPrefix = "aihubkit:"

var errHubConnection = errors.New("无法连接平台，请稍后重试")

type hubConnectionError struct {
	phase     string
	retryable bool
}

func (e *hubConnectionError) Error() string {
	return fmt.Sprintf("%s（%s）", errHubConnection, e.phase)
}

type User struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Group       string `json:"group"`
	Status      int    `json:"status"`
	Quota       int64  `json:"quota"`
	UsedQuota   int64  `json:"used_quota"`
}
type Group struct {
	Desc  string  `json:"desc"`
	Ratio float64 `json:"ratio"`
}
type CatalogModel struct {
	ID        string   `json:"id"`
	Type      string   `json:"model_type"`
	Endpoints []string `json:"supported_endpoint_types"`
}
type client struct {
	origin       string
	platformName string
	brand        *branding.Service
	http         *http.Client
	uploadHTTP   *http.Client
}

func newClient() *client {
	h := outbound.OutboundHTTPClient(45 * time.Second)
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	uploadHTTP := outbound.OutboundHTTPClient(10 * time.Minute)
	uploadHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client{origin: Origin, platformName: "aihubkit", http: h, uploadHTTP: uploadHTTP}
}

func (c *client) upstreamOrigin() string {
	if c.brand != nil && c.brand.Current().ID != "" {
		return c.brand.Current().APIOrigin
	}
	return c.origin
}

func (c *client) accountOrigin() string {
	if c.brand != nil && c.brand.Current().ID != "" {
		return c.brand.Current().Origin
	}
	return c.origin
}

func (c *client) displayName() string {
	if c.brand != nil && c.brand.Current().ID != "" {
		return c.brand.Current().PlatformName
	}
	if c.platformName != "" {
		return c.platformName
	}
	return "aihubkit"
}

// Remote error bodies and transport errors may contain credentials or signed
// URLs. Only bounded, structured data from successful responses crosses here.
func (c *client) call(ctx context.Context, method, path string, payload any, token string, userID int, result any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.upstreamOrigin()+path, body)
	if err != nil {
		return errors.New("平台请求无效")
	}
	r.Header.Set("Accept", "application/json")
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if userID > 0 {
		r.Header.Set("New-Api-User", strconv.Itoa(userID))
	}
	var trace outbound.TransportTrace
	resp, err := c.http.Do(trace.Attach(r))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, phase := trace.Result(err)
		label := map[string]string{"dns": "域名解析失败", "connect": "网络或代理连接失败", "tls": "TLS 连接失败", "request": "发送请求时连接中断", "response_wait": "等待响应时连接中断", "response": "读取响应时连接中断"}[phase]
		if label == "" {
			label = "网络连接失败"
		}
		var certificate *tls.CertificateVerificationError
		return &hubConnectionError{phase: label, retryable: !errors.As(err, &certificate)}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("平台拒绝请求，请检查账号权限、凭据和额度")
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Object  string          `json:"object"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope); err != nil {
		return errors.New("平台响应格式无效")
	}
	if !envelope.Success && envelope.Object != "list" {
		return errors.New("平台操作未成功，请检查输入、账号状态或稍后重试")
	}
	if result != nil && json.Unmarshal(envelope.Data, result) != nil {
		return errors.New("平台响应数据无效")
	}
	return nil
}

func (c *client) login(ctx context.Context, username, password string) (User, string, error) {
	// The dashboard session is used only to acquire the management token. It is
	// never copied to the WebView or the general-purpose object upload client.
	jar, _ := cookiejar.New(nil)
	loginHTTP := *c.http
	loginHTTP.Jar = jar
	loginClient := *c
	loginClient.http = &loginHTTP
	var user User
	err := loginClient.call(ctx, "POST", "/api/user/login", map[string]string{"username": username, "password": password}, "", 0, &user)
	if err != nil {
		return User{}, "", err
	}
	if user.ID <= 0 || user.Status != 1 {
		return User{}, "", errors.New("账号不可用，或该账号需要其他登录验证")
	}
	var token string
	if err := loginClient.call(ctx, "GET", "/api/user/token", nil, "", user.ID, &token); err != nil {
		return User{}, "", err
	}
	if strings.TrimSpace(token) == "" {
		return User{}, "", errors.New("未取得账号管理凭据")
	}
	return user, token, nil
}
