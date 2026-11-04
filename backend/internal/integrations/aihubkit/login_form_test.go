package aihubkit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/localcrypto"
)

func TestRejectedLoginDoesNotReplaceRememberedPassword(t *testing.T) {
	h, err := Open(context.Background(), bootstrap.Config{Profile: bootstrap.ProfileDesktop, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	h.client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/user/login" {
			t.Error("rejected login attempted another upstream operation")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"success":false,"message":"invalid credentials"}`))}, nil
	})}
	if err := h.saveLoginForm("fixture-user", "fixture-password", true); err != nil {
		t.Fatal(err)
	}
	for _, remember := range []string{"true", "false"} {
		r := httptest.NewRequest("POST", apiPrefix+"login", strings.NewReader(`{"username":"wrong","password":"incorrect","rememberCredentials":`+remember+`}`))
		r.Header.Set("X-Aihubkit-Client", "1")
		r.Header.Set("X-Desktop-Token", h.LaunchToken())
		w := httptest.NewRecorder()
		h.Handler().ServeHTTP(w, r)
		if w.Code != 401 || h.Ready() {
			t.Fatal("rejected login opened a workspace")
		}
		saved, err := h.loadLoginForm()
		if err != nil || saved == nil || saved.Password != "fixture-password" {
			t.Fatal("rejected login replaced the saved credentials")
		}
	}
}

func TestRememberedLoginIsEncryptedAndOriginScoped(t *testing.T) {
	h := &Host{cfg: bootstrap.Config{DataDir: t.TempDir()}, client: &client{origin: Origin}}
	if err := h.saveLoginForm("fixture-user", "fixture-password", true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.loginFormDir(), "credentials.enc")
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), localcrypto.Prefix) || strings.Contains(string(data), "fixture") {
		t.Fatal("login credentials were not encrypted")
	}
	for _, path := range []string{path, filepath.Join(h.loginFormDir(), ".settings-key")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("credential file permissions are not private")
		}
	}
	saved, err := h.loadLoginForm()
	if err != nil || saved == nil || saved.Username != "fixture-user" || saved.Password != "fixture-password" {
		t.Fatal("autofill did not recover the exact credentials")
	}
	h.client.origin = "https://other.example"
	if saved, err := h.loadLoginForm(); err != nil || saved != nil {
		t.Fatal("credentials crossed platform origins")
	}
	h.client.origin = Origin
	if err := os.Remove(filepath.Join(h.loginFormDir(), ".settings-key")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.loadLoginForm(); err == nil {
		t.Fatal("missing encryption key was ignored")
	}
	if err := h.saveLoginForm("", "", false); err != nil {
		t.Fatal(err)
	}
	if saved, err := h.loadLoginForm(); err != nil || saved != nil {
		t.Fatal("forget did not remove remembered credentials")
	}
}

func TestLoginFormRequiresDesktopAuthorizationAndDoesNotLogin(t *testing.T) {
	for _, profile := range []bootstrap.Profile{bootstrap.ProfileDesktop, bootstrap.ProfileServer} {
		t.Run(string(profile), func(t *testing.T) {
			h, err := Open(context.Background(), bootstrap.Config{Profile: profile, DataDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close(context.Background())
			if err := h.saveLoginForm("fixture-user", "fixture-password", true); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				method, path, token, origin, client string
				status                              int
			}{
				{"POST", "login-form", "", "", "1", 403},
				{"POST", "login-form", "wrong", "", "1", 403},
				{"POST", "login-form", h.LaunchToken(), "https://attacker.example", "1", 403},
				{"POST", "login-form", h.LaunchToken(), "", "", 400},
				{"GET", "login-form", h.LaunchToken(), "", "1", 400},
				{"POST", "login-form", h.LaunchToken(), "", "1", 200},
				{"GET", "session", h.LaunchToken(), "", "1", 200},
				{"DELETE", "login-form", h.LaunchToken(), "", "1", 200},
				{"POST", "login-form", h.LaunchToken(), "", "1", 200},
			} {
				r := httptest.NewRequest(tc.method, apiPrefix+tc.path, nil)
				r.Header.Set("X-Desktop-Token", tc.token)
				r.Header.Set("X-Aihubkit-Client", tc.client)
				r.Header.Set("Origin", tc.origin)
				w := httptest.NewRecorder()
				h.Handler().ServeHTTP(w, r)
				status := tc.status
				if profile == bootstrap.ProfileServer && tc.path == "login-form" {
					status = 403
				}
				if w.Code != status {
					t.Fatalf("%s %s: got status %d, want %d", tc.method, tc.path, w.Code, status)
				}
				if (status != 200 || tc.path == "session") && strings.Contains(w.Body.String(), "fixture-password") {
					t.Fatal("password exposed outside the authorized autofill response")
				}
				if w.Header().Get("Cache-Control") != "no-store" || h.Ready() {
					t.Fatal("autofill was cacheable or authenticated the account")
				}
			}
			if profile == bootstrap.ProfileDesktop {
				if saved, err := h.loadLoginForm(); err != nil || saved != nil {
					t.Fatal("forget endpoint did not clear credentials")
				}
			}
		})
	}
}
