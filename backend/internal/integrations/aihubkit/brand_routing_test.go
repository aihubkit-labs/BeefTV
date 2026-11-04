package aihubkit

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/integrations/aihubkit/branding"
)

func TestAllAccountAndRelayRequestsUseStatusAPIAddress(t *testing.T) {
	brand := branding.New(branding.Brand{ID: "customer", Origin: "https://brand.example", APIOrigin: "https://api.example", PlatformName: "Customer"})
	c := newClient()
	c.brand = brand
	c.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.example" {
			t.Errorf("request sent to branding origin: %s", r.URL.Host)
		}
		body := `{"success":true,"data":null}`
		switch r.URL.Path {
		case "/api/user/login":
			body = `{"success":true,"data":{"id":1,"status":1}}`
		case "/api/user/token":
			body = `{"success":true,"data":"management-fixture"}`
		case "/v1/models":
			body = `{"object":"list","data":[]}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	if _, _, err := c.login(context.Background(), "fixture", "fixture-password"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/user/self", "/api/user/self/groups", "/api/log/self", "/api/token/", "/v1/models", "/v1/temporary-files"} {
		if err := c.call(context.Background(), "GET", path, nil, "fixture-token", 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	a := &account{client: c, groups: map[string]Group{"group": {}}, keys: map[string]string{"group": "inference-fixture"}}
	origin, _, handled, err := a.resolve("aihubkit:group")
	if err != nil || !handled || origin != "https://api.example" {
		t.Fatal(origin, handled, err)
	}
	if c.accountOrigin() != "https://brand.example" {
		t.Fatal("storage identity must remain bound to the brand")
	}
}
