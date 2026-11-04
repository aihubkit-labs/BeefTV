package aihubkit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/generation"
	"infinite-canvas/backend/internal/workspace"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDashboardLoginUsesPrivateCookieJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("dashboard login unexpectedly sent bearer token")
		}
		switch r.URL.Path {
		case "/api/user/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "dashboard-secret", Path: "/"})
			io.WriteString(w, `{"success":true,"data":{"id":7,"status":1,"group":"video","username":"creator"}}`)
		case "/api/user/token":
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value != "dashboard-secret" || r.Header.Get("New-Api-User") != "7" {
				t.Error("missing login cookie or user header")
			}
			io.WriteString(w, `{"success":true,"data":"management-secret"}`)
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	c := &client{origin: server.URL, http: server.Client()}
	user, token, err := c.login(context.Background(), "creator", "password")
	if err != nil || user.ID != 7 || token != "management-secret" {
		t.Fatalf("login: user=%+v err=%v", user, err)
	}
	if c.http.Jar != nil {
		t.Fatal("dashboard cookies escaped the login client")
	}
}

func TestAccountKeysAreEncryptedAndReusedPerGroup(t *testing.T) {
	var created atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token/" || r.Header.Get("Authorization") != "Bearer management-secret" || r.Header.Get("New-Api-User") != "7" {
			t.Error("invalid token creation request")
		}
		created.Add(1)
		io.WriteString(w, `{"success":true,"data":{"key":"inference-secret"}}`)
	}))
	defer server.Close()
	a := &account{dir: t.TempDir(), client: &client{origin: server.URL, http: server.Client()}, user: User{ID: 7}, token: "management-secret", groups: map[string]Group{"video": {}}, keys: map[string]string{}}
	if err := a.ensureKey(context.Background(), "unknown"); err == nil {
		t.Fatal("unauthorized group accepted")
	}
	if err := a.ensureKey(context.Background(), "video"); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureKey(context.Background(), "video"); err != nil || created.Load() != 1 {
		t.Fatalf("key was recreated: %v", err)
	}
	_, key, handled, err := a.resolve("aihubkit:video")
	if err != nil || !handled || key != "sk-inference-secret" {
		t.Fatal("inference key resolution failed")
	}
	b, err := os.ReadFile(filepath.Join(a.dir, "aihubkit-keys.enc"))
	if err != nil || strings.Contains(string(b), "inference-secret") || strings.Contains(string(b), "management-secret") {
		t.Fatal("credentials stored in plaintext")
	}
	if accountScope(Origin, 7) == accountScope(Origin, 8) || accountScope(Origin, 7) == accountScope("https://other.example", 7) {
		t.Fatal("account namespace collision")
	}
}

func TestUploadUsesGeneralFileTypesAndNoAccountCredentialsAtObjectStore(t *testing.T) {
	for _, mime := range []string{"image/png", "video/mp4", "audio/wav", "application/pdf"} {
		t.Run(mime, func(t *testing.T) {
			c := newClient()
			c.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/v1/temporary-files" || r.Header.Get("Authorization") != "Bearer management-secret" {
					t.Fatal("invalid ticket request")
				}
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Fatal(err)
				}
				if input["content_type"] != mime || input["size"] != float64(3) {
					t.Fatal("file metadata changed")
				}
				ticket, _ := json.Marshal(map[string]any{"success": true, "data": uploadTicket{UploadURL: "https://8.8.8.8/put?signature=private", URL: "https://8.8.8.8/get?signature=private", ExpiresIn: 600, Headers: map[string]string{"Content-Type": mime}}})
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(ticket)))}, nil
			})}
			c.uploadHTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				if r.Method != "PUT" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Content-Type") != mime || r.ContentLength != 3 || string(b) != "abc" {
					t.Fatal("object PUT leaked credentials or changed file")
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("ticket deadline missing")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			link, deadline, err := c.upload(context.Background(), "management-secret", "file", mime, 3, strings.NewReader("abc"))
			if err != nil || link == "" || time.Until(deadline) < 9*time.Minute {
				t.Fatalf("upload: %v", err)
			}
		})
	}
}

func TestUploadFollowsOutboundAddressPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, allowPrivate, uploadURL, readURL string
		allowed                                bool
	}{
		{"local allowed", "true", "http://127.0.0.1/put", "http://127.0.0.1/get", true},
		{"strict upload", "false", "https://127.0.0.1/put", "https://8.8.8.8/get", false},
		{"strict read", "false", "https://8.8.8.8/put", "https://127.0.0.1/get", false},
		{"public HTTPS", "false", "https://8.8.8.8/put", "https://8.8.8.8/get", true},
		{"link local", "true", "http://169.254.169.254/put", "https://8.8.8.8/get", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", tc.allowPrivate)
			t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "")
			c := newClient()
			c.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				ticket, _ := json.Marshal(map[string]any{"success": true, "data": uploadTicket{UploadURL: tc.uploadURL, URL: tc.readURL, ExpiresIn: 600}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(ticket)))}, nil
			})}
			putCalled := false
			c.uploadHTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				putCalled = true
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			link, _, err := c.upload(context.Background(), "secret", "file", "video/mp4", 1, strings.NewReader("a"))
			if (err == nil) != tc.allowed || putCalled != tc.allowed {
				t.Fatalf("allowed=%v put=%v err=%v", tc.allowed, putCalled, err)
			}
			if tc.allowed && link != tc.readURL {
				t.Fatal("read URL changed")
			}
		})
	}
}

func TestUploadRejectsExpiredReferenceBatches(t *testing.T) {
	s := &accountStorage{account: &account{}, expiresAt: time.Now().Add(5 * time.Second)}
	if s.ValidateReferences(context.Background()) == nil {
		t.Fatal("accepted an expiring earlier reference")
	}
	s.expiresAt = time.Now().Add(time.Minute)
	if err := s.ValidateReferences(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAccountReferenceStorageSelectionOnlyDependsOnUploadPreference(t *testing.T) {
	a := &account{client: &client{origin: Origin}}
	for _, enabled := range []bool{true, false} {
		a.uploadToAIHubkit.Store(enabled)
		for _, config := range []generation.Config{
			{BaseURL: Origin + "/v1"},
			{CredentialRef: credentialPrefix + "video"},
			{BaseURL: "https://other.example/v1"},
		} {
			if (a.referenceStorage(config) != nil) != enabled {
				t.Fatalf("storage selection ignored preference=%v for %+v", enabled, config)
			}
		}
	}
}

func TestHostRequiresLoginAndRejectsForeignOrigins(t *testing.T) {
	h, err := Open(context.Background(), bootstrap.Config{Profile: bootstrap.ProfileServer, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close(context.Background())
	for _, tc := range []struct {
		path, origin string
		status       int
	}{
		{"/api/workspace/bootstrap", "", 401},
		{apiPrefix + "session", "", 200},
		{apiPrefix + "session", "https://attacker.example", 403},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: status %d", tc.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "management-secret") {
			t.Fatal("credential exposed")
		}
	}
}

func TestAccountWorkspaceSurvivesLogoutAndIsIsolatedFromOtherAccounts(t *testing.T) {
	for _, profile := range []bootstrap.Profile{bootstrap.ProfileServer, bootstrap.ProfileDesktop} {
		t.Run(string(profile), func(t *testing.T) { testAccountWorkspace(t, profile) })
	}
}

func testAccountWorkspace(t *testing.T, profile bootstrap.Profile) {
	var created atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/login":
			var input struct{ Username string }
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			id := 7
			if input.Username == "second" {
				id = 8
			}
			http.SetCookie(w, &http.Cookie{Name: "dashboard", Value: strconv.Itoa(id), Path: "/"})
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": User{ID: id, Username: input.Username, Group: "video", Status: 1}})
		case "/api/user/token":
			cookie, err := r.Cookie("dashboard")
			if err != nil || cookie.Value != r.Header.Get("New-Api-User") {
				t.Error("login cookie and user ID must match")
			}
			io.WriteString(w, `{"success":true,"data":"management-secret"}`)
		case "/api/user/self/groups":
			io.WriteString(w, `{"success":true,"data":{"video_官key":{"desc":"Video","ratio":1}}}`)
		case "/api/token/":
			created.Add(1)
			io.WriteString(w, `{"success":true,"data":{"key":"inference-secret"}}`)
		case "/v1/models":
			io.WriteString(w, `{"object":"list","data":[{"id":"test-video","supported_endpoint_types":["openai-video"]}]}`)
		default:
			t.Errorf("unexpected remote call: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	h, err := Open(context.Background(), bootstrap.Config{Profile: profile, DataDir: t.TempDir(), AutoMigrate: true, ShutdownTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	h.client = &client{origin: server.URL, http: server.Client()}
	// Reproduce an account that saved settings before default-model onboarding
	// existed. Login must repair it before the official runtime reads its config.
	store, err := workspace.NewProviderConfig(filepath.Join(h.cfg.DataDir, "aihubkit", "accounts", accountScope(server.URL, 7)))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveLocalModelConfig([]byte(`{"channels":[]}`)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	request := func(method, path, body, session, cookie string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://127.0.0.1:54321"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aihubkit-Client", "1")
		if profile == bootstrap.ProfileDesktop {
			r.Header.Set("Origin", "wails://wails")
			r.Header.Set("X-Desktop-Token", h.LaunchToken())
			r.Header.Set("X-Beeftv-UI-Bootstrap", h.UIBootstrapToken())
		}
		r.Header.Set(sessionHeader, session)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.Handler().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: status=%d body=%s", method, path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "management-secret") || strings.Contains(w.Body.String(), "inference-secret") {
			t.Fatal("workspace response exposed remote credentials")
		}
		return w
	}
	login := func(username string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"username": username, "password": "password", "rememberCredentials": profile == bootstrap.ProfileDesktop && username == "first"})
		w := request("POST", apiPrefix+"login", string(body), "", "", 200)
		if strings.Contains(w.Body.String(), `"password"`) {
			t.Fatal("login session exposed the password")
		}
		var envelope struct {
			Data struct {
				Session string `json:"session"`
				Scope   string `json:"scope"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Data.Session == "" || envelope.Data.Scope == "" {
			t.Fatal("missing account session/scope")
		}
		return envelope.Data.Session
	}
	first := login("first")
	if profile == bootstrap.ProfileDesktop {
		request("POST", apiPrefix+"login-form", `{}`, "", "", 409)
	}
	if !strings.Contains(request("GET", apiPrefix+"session", "", first, "", 200).Body.String(), `"uploadToAIHubkit":true`) {
		t.Fatal("login did not default to platform uploads")
	}
	request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":false}`, first, "", 200)
	request("GET", "/api/workspace/bootstrap", "", first, "", 200)
	modelConfig := request("GET", "/api/workspace/model-config", "", first, "", 200)
	var seeded struct {
		Data struct {
			Config struct {
				VideoModel string `json:"videoModel"`
				Channels   []struct {
					ID            string `json:"id"`
					InterfaceType string `json:"interfaceType"`
					Profiles      []struct {
						Model      string `json:"model"`
						Capability string `json:"capability"`
						Protocol   string `json:"protocol"`
					} `json:"modelProfiles"`
				} `json:"channels"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelConfig.Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}
	if seeded.Data.Config.VideoModel != "aihubkit-video_官key::test-video" {
		t.Fatal("default group video model was not selected")
	}
	found := false
	for _, channel := range seeded.Data.Config.Channels {
		if channel.ID != "aihubkit-video_官key" {
			continue
		}
		found = true
		if channel.InterfaceType != "" || len(channel.Profiles) != 1 || channel.Profiles[0].Model != "test-video" || channel.Profiles[0].Capability != "video" || channel.Profiles[0].Protocol != "aihubkit-video" {
			t.Fatal("default group must use model protocols without a channel-wide fallback")
		}
	}
	if !found {
		t.Fatal("default group channel is missing")
	}
	request("POST", apiPrefix+"default-model-config", `{}`, "", first, 409)
	reset := request("POST", apiPrefix+"default-model-config", `{}`, first, "", 200)
	if !strings.Contains(reset.Body.String(), `"videoModel":"aihubkit-video_官key::test-video"`) || !strings.Contains(reset.Body.String(), `"warnings":[`) {
		t.Fatal("manual initialization did not use the onboarding model rules")
	}
	unchanged := request("GET", "/api/workspace/model-config", "", first, "", 200)
	if unchanged.Body.String() != modelConfig.Body.String() {
		t.Fatal("preparing default models changed saved configuration before UI commit")
	}
	w := request("POST", "/api/projects", `{"name":"first account project"}`, first, "", 200)
	var project struct {
		Data struct {
			Project struct{ ID string } `json:"project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &project); err != nil || project.Data.Project.ID == "" {
		t.Fatal("missing created project")
	}
	id := project.Data.Project.ID
	request("POST", "/api/projects", `{"name":"cookie only write"}`, "", first, 409)
	request("POST", apiPrefix+"logout", `{}`, first, "", 200)
	request("GET", "/api/projects/"+id, "", first, "", 401)
	if profile == bootstrap.ProfileDesktop {
		autofill := request("POST", apiPrefix+"login-form", `{}`, "", "", 200)
		if !strings.Contains(autofill.Body.String(), `"username":"first"`) || !strings.Contains(autofill.Body.String(), `"password":"password"`) || h.Ready() {
			t.Fatal("autofill did not retain the form or opened a workspace without login")
		}
	}
	second := login("second")
	if !h.account.uploadToAIHubkit.Load() {
		t.Fatal("another account inherited the first account's upload preference")
	}
	request("GET", "/api/projects/"+id, "", second, "", 404)
	request("POST", "/api/projects", `{"name":"stale tab"}`, first, second, 401)
	request("POST", apiPrefix+"logout", `{}`, second, "", 200)
	if profile == bootstrap.ProfileDesktop && !strings.Contains(request("POST", apiPrefix+"login-form", `{}`, "", "", 200).Body.String(), `"data":null`) {
		t.Fatal("login without remembering retained an earlier password")
	}
	again := login("first")
	if !strings.Contains(request("GET", apiPrefix+"session", "", again, "", 200).Body.String(), `"uploadToAIHubkit":false`) {
		t.Fatal("relogin lost the saved upload preference")
	}
	if first == again {
		t.Fatal("logout did not rotate the local session")
	}
	request("GET", "/api/projects/"+id, "", again, "", 200)
	if created.Load() != 2 {
		t.Fatal("relogin recreated persisted group credentials")
	}
	accountDir := h.account.dir
	activeRuntime, launchToken, uiToken := h.active, h.LaunchToken(), h.UIBootstrapToken()
	commitCanvas := func(revision int) {
		t.Helper()
		if profile != bootstrap.ProfileDesktop {
			return
		}
		body := `{"opId":"binding-save-` + strconv.Itoa(revision) + `","params":{"canvasId":"binding-canvas","expectedRevision":` + strconv.Itoa(revision) + `,"document":{"title":"binding save","nodes":[{"id":"image","type":"image"},{"id":"video","type":"video"}],"connections":[]}}}`
		request("POST", "/api/ops/canvas.document.commit", body, again, "", 200)
		saved := request("GET", "/api/canvas-projects/binding-canvas", "", again, "", 200)
		if !strings.Contains(saved.Body.String(), `"revision":`+strconv.Itoa(revision+1)) || !strings.Contains(saved.Body.String(), `"id":"image"`) || !strings.Contains(saved.Body.String(), `"id":"video"`) {
			t.Fatal("canvas document was not persisted")
		}
	}
	if profile == bootstrap.ProfileDesktop {
		request("PUT", "/api/canvas-projects/binding-canvas", `{"project":{"id":"binding-canvas","title":"binding save","revision":0,"nodes":[],"connections":[]}}`, again, "", 200)
		commitCanvas(1)
	}
	beforeConfig := request("GET", "/api/workspace/model-config", "", again, "", 200).Body.String()
	request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":true}`, again, "", 200)
	request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":false}`, "", again, 409)
	for _, body := range []string{`{}`, `{"uploadToAIHubkit":null}`, `{"uploadToAIHubkit":"false"}`} {
		request("POST", apiPrefix+"upload-preference", body, again, "", 400)
	}
	request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":false}`, "invalid", "", 401)
	if !h.account.uploadToAIHubkit.Load() {
		t.Fatal("rejected request changed upload preference")
	}
	request("GET", "/api/projects/"+id, "", again, "", 200)
	updated := request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":false}`, again, "", 200)
	if !strings.Contains(updated.Body.String(), `"uploadToAIHubkit":false`) || !strings.Contains(updated.Body.String(), `"authenticated":true`) {
		t.Fatal("upload preference must not change login state")
	}
	if !strings.Contains(request("GET", apiPrefix+"session", "", again, "", 200).Body.String(), `"uploadToAIHubkit":false`) {
		t.Fatal("refresh lost the upload preference")
	}
	if h.active != activeRuntime || h.session != again || h.LaunchToken() != launchToken || h.UIBootstrapToken() != uiToken {
		t.Fatal("upload preference changed the workspace or desktop authorization")
	}
	commitCanvas(2)
	request("GET", "/api/projects/"+id, "", again, "", 200)
	request("POST", apiPrefix+"catalog", `{"group":"video_官key"}`, again, "", 200)
	request("POST", apiPrefix+"default-model-config", `{}`, again, "", 200)
	if _, key, handled, err := h.account.resolve("aihubkit:video_官key"); err != nil || key == "" || !handled {
		t.Fatal("upload preference changed imported model credentials")
	}
	localProject := request("POST", "/api/projects", `{"name":"project with native storage"}`, again, "", 200)
	if err := json.Unmarshal(localProject.Body.Bytes(), &project); err != nil {
		t.Fatal(err)
	}
	localID := project.Data.Project.ID
	if request("GET", "/api/workspace/model-config", "", again, "", 200).Body.String() != beforeConfig {
		t.Fatal("upload preference rewrote model configuration")
	}
	_, localDir, release, err := h.AcquireWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	release()
	if localDir != accountDir {
		t.Fatal("upload preference changed the native workspace")
	}
	updated = request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":true}`, again, "", 200)
	if !strings.Contains(updated.Body.String(), `"uploadToAIHubkit":true`) {
		t.Fatal("enabling platform uploads did not take effect")
	}
	commitCanvas(3)
	if _, key, _, err := h.account.resolve("aihubkit:video_官key"); err != nil || key == "" {
		t.Fatal("enabling platform uploads changed credentials")
	}
	request("GET", "/api/projects/"+id, "", again, "", 200)
	request("GET", "/api/projects/"+localID, "", again, "", 200)
	request("POST", apiPrefix+"upload-preference", `{"uploadToAIHubkit":false}`, again, "", 200)
	cfg := h.cfg
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	h, err = Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.client = &client{origin: server.URL, http: server.Client()}
	request("GET", "/api/projects/"+localID, "", "", "", 401)
	if profile == bootstrap.ProfileDesktop {
		autofill := request("POST", apiPrefix+"login-form", `{}`, "", "", 200)
		if !strings.Contains(autofill.Body.String(), `"password":"password"`) || h.Ready() {
			t.Fatal("restart must restore the form without authenticating")
		}
	}
	restarted := login("first")
	state := request("GET", apiPrefix+"session", "", restarted, "", 200)
	if !strings.Contains(state.Body.String(), `"uploadToAIHubkit":false`) {
		t.Fatal("restart lost the saved upload preference")
	}
	request("GET", "/api/projects/"+localID, "", restarted, "", 200)
	request("GET", "/api/projects/"+id, "", restarted, "", 200)
}
