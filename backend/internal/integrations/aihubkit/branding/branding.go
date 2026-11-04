// Package branding owns domain-bound platform metadata independently of login.
package branding

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"infinite-canvas/backend/internal/outbound"
)

// Snapshot is injected by the brand build entry. It contains no credentials.
var Snapshot string

type Brand struct {
	ID           string `json:"id"`
	Origin       string `json:"origin"`
	PlatformName string `json:"platformName"`
	AppName      string `json:"appName"`
	LogoURL      string `json:"logoUrl"`
	WebsiteURL   string `json:"websiteUrl"`
	WeChatURL    string `json:"wechatUrl"`
	APIOrigin    string `json:"apiOrigin"`
	BundledLogo  string `json:"bundledLogo,omitempty"`
}

type Status struct {
	SystemName       string `json:"system_name"`
	Logo             string `json:"logo"`
	ServerAddress    string `json:"server_address"`
	WeChatQRCode     string `json:"wechat_qrcode"`
	APIServerAddress string `json:"api_server_address"`
}

func HTTPSAddress(raw string, originOnly bool) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(raw, "\r\n\\") || (originOnly && (u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/"))) {
		return "", errors.New("品牌配置必须使用有效的 HTTPS 地址")
	}
	if originOnly {
		return "https://" + u.Host, nil
	}
	return u.String(), nil
}

func FromStatus(base Brand, status Status) (Brand, error) {
	name := strings.TrimSpace(status.SystemName)
	if name == "" || len([]rune(name)) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		return Brand{}, errors.New("平台名称无效")
	}
	var err error
	base.PlatformName, base.AppName = name, name+"TV"
	if base.APIOrigin, err = HTTPSAddress(status.APIServerAddress, true); err != nil {
		return Brand{}, err
	}
	if base.WebsiteURL, err = HTTPSAddress(status.ServerAddress, false); err != nil {
		return Brand{}, err
	}
	if base.LogoURL, err = HTTPSAddress(status.Logo, false); err != nil {
		return Brand{}, err
	}
	base.WeChatURL = ""
	if status.WeChatQRCode != "" {
		if base.WeChatURL, err = HTTPSAddress(status.WeChatQRCode, false); err != nil {
			return Brand{}, err
		}
	}
	return base, nil
}

func Embedded() (Brand, error) {
	if Snapshot == "" {
		return Brand{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(Snapshot)
	if err != nil {
		return Brand{}, errors.New("安装包品牌配置无效")
	}
	var brand Brand
	if json.Unmarshal(raw, &brand) != nil || brand.ID == "" {
		return Brand{}, errors.New("安装包品牌配置无效")
	}
	brand.Origin, err = HTTPSAddress(brand.Origin, true)
	if err != nil {
		return Brand{}, err
	}
	return FromStatus(brand, Status{brand.PlatformName, brand.LogoURL, brand.WebsiteURL, brand.WeChatURL, brand.APIOrigin})
}

type Service struct {
	mu      sync.Mutex
	refresh sync.Mutex
	brand   Brand
	checked time.Time
	http    *http.Client
}

func New(brand Brand) *Service {
	client := outbound.OutboundHTTPClient(10 * time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Service{brand: brand, http: client}
}

func (s *Service) Current() Brand { s.mu.Lock(); defer s.mu.Unlock(); return s.brand }

// Failed refreshes preserve this package's own snapshot; they cannot switch brands.
func (s *Service) Refresh(ctx context.Context) (Brand, error) {
	s.refresh.Lock()
	defer s.refresh.Unlock()
	s.mu.Lock()
	current := s.brand
	if s.brand.ID == "" || time.Since(s.checked) < time.Minute {
		s.mu.Unlock()
		return current, nil
	}
	s.checked = time.Now()
	s.mu.Unlock()
	r, err := http.NewRequestWithContext(ctx, "GET", current.Origin+"/api/status", nil)
	if err != nil {
		return current, err
	}
	r.Header.Set("Accept", "application/json")
	response, err := s.http.Do(r)
	if err != nil {
		return current, errors.New("品牌信息暂时无法刷新")
	}
	defer response.Body.Close()
	var envelope struct {
		Success bool   `json:"success"`
		Data    Status `json:"data"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope) != nil || !envelope.Success {
		return current, errors.New("品牌信息响应无效")
	}
	brand, err := FromStatus(current, envelope.Data)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.brand = brand
	}
	return s.brand, err
}
