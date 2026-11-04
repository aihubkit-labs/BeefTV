package branding

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRefreshKeepsBindingAndLastValidSnapshot(t *testing.T) {
	valid := true
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/status" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("metadata request crossed an account boundary")
		}
		if !valid {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": Status{SystemName: "客户平台", Logo: "https://cdn.example/logo.png", ServerAddress: "https://site.example", WeChatQRCode: "https://cdn.example/qr.png", APIServerAddress: "https://api.example"}})
	}))
	defer server.Close()
	s := New(Brand{ID: "customer", Origin: server.URL, PlatformName: "old", BundledLogo: "/aihubkit-brand/logo.png"})
	s.http = server.Client()
	brand, err := s.Refresh(context.Background())
	if err != nil || brand.ID != "customer" || brand.Origin != server.URL || brand.AppName != "客户平台TV" || brand.APIOrigin != "https://api.example" {
		t.Fatalf("brand=%+v err=%v", brand, err)
	}
	_, _ = s.Refresh(context.Background())
	if requests != 1 {
		t.Fatal("refresh was not rate limited")
	}
	valid, s.checked = false, time.Time{}
	got, err := s.Refresh(context.Background())
	if err == nil || got != brand {
		t.Fatalf("failed refresh lost the snapshot: %+v %v", got, err)
	}
}

func TestEmbeddedSnapshotAndInvalidEndpoints(t *testing.T) {
	previous := Snapshot
	t.Cleanup(func() { Snapshot = previous })
	valid := Brand{ID: "brand", Origin: "https://brand.example", PlatformName: "Brand", LogoURL: "https://cdn.example/logo.png", WebsiteURL: "https://site.example", APIOrigin: "https://api.example"}
	for _, origin := range []string{"https://api.example", "http://api.example", "https://user:password@api.example", "https://api.example/path", "https://api.example?q=1"} {
		brand := valid
		brand.APIOrigin = origin
		body, _ := json.Marshal(brand)
		Snapshot = base64.StdEncoding.EncodeToString(body)
		got, err := Embedded()
		if origin == valid.APIOrigin {
			if err != nil || got.AppName != "BrandTV" {
				t.Fatal(got, err)
			}
		} else if err == nil {
			t.Fatalf("accepted unsafe API origin %s", origin)
		}
	}
}
