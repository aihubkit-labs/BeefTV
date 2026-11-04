package aihubkit

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/generation"
	"infinite-canvas/backend/internal/referencestorage"
)

func TestUploadPreferenceDoesNotChangeModelCredentials(t *testing.T) {
	a := &account{client: &client{origin: "https://example.invalid"}, token: "management", groups: map[string]Group{"group": {}}, keys: map[string]string{"group": "inference"}}
	a.uploadToAIHubkit.Store(true)
	managed := generation.Config{CredentialRef: "aihubkit:group"}
	manual := generation.Config{BaseURL: "https://other.invalid"}
	storage := a.referenceStorage(managed)
	if storage == nil || a.referenceStorage(manual) == nil {
		t.Fatal("platform storage must apply to all channels when selected")
	}
	a.uploadToAIHubkit.Store(false)
	if a.referenceStorage(managed) != nil || a.referenceStorage(manual) != nil || a.referenceStorage(generation.Config{BaseURL: a.client.origin + "/v1"}) != nil {
		t.Fatal("disabled platform storage still overrides native storage")
	}
	if enabled, err := storage.Enabled(); err != nil || enabled {
		t.Fatal("previously selected platform storage remained enabled")
	}
	if _, err := storage.Upload(context.Background(), "local", "reference"); err == nil {
		t.Fatal("a previously selected batch started a platform upload after disabling it")
	}
	if err := storage.(*accountStorage).ValidateReferences(context.Background()); err != nil {
		t.Fatal("changing upload preference invalidated existing reference links")
	}
	if _, key, handled, err := a.resolve("aihubkit:group"); err != nil || key != "inference" || !handled {
		t.Fatal("disabling platform uploads lost imported model credentials")
	}
	for _, ref := range []string{"aihubkit:unknown", "aihubkit:"} {
		if _, key, handled, err := a.resolve(ref); err == nil || key != "" || !handled {
			t.Fatal("upload preference bypassed group credential validation")
		}
	}
	if _, _, handled, err := a.resolve("custom"); err != nil || handled {
		t.Fatal("upload preference restricted a native custom provider")
	}
	a.uploadToAIHubkit.Store(true)
	if _, key, handled, err := a.resolve("aihubkit:group"); err != nil || key != "inference" || !handled {
		t.Fatal("enabling platform uploads changed model credentials")
	}
	if a.referenceStorage(managed) == nil {
		t.Fatal("enabling platform uploads did not take effect")
	}
}

func TestPlatformStorageUploadsOwnedReferencesForAllChannels(t *testing.T) {
	c := newClient()
	issued, uploaded := 0, 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		issued++
		if r.URL.Path != "/v1/temporary-files" || r.Header.Get("Authorization") != "Bearer management" {
			t.Fatal("invalid reference ticket request")
		}
		ticket, _ := json.Marshal(map[string]any{"success": true, "data": uploadTicket{
			UploadURL: "https://8.8.8.8/put", URL: "https://8.8.8.8/reference.png", ExpiresIn: 600,
		}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(ticket))}, nil
	})
	var content bytes.Buffer
	if err := png.Encode(&content, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	c.uploadHTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		uploaded++
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, content.Bytes()) || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("reference upload changed the file or leaked credentials")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	a := &account{client: c, token: "management"}
	a.uploadToAIHubkit.Store(true)
	runtime, err := bootstrap.Open(context.Background(), bootstrap.Config{
		Profile: bootstrap.ProfileDesktop, DataDir: t.TempDir(), AutoMigrate: true, ConfigureService: a.configure,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	resource, err := a.service.UploadResourceFile("local", "reference.png", int64(content.Len()), "image", 1, 1, 0, bytes.NewReader(content.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, storage := range []generation.ReferenceStoragePort{
		a.referenceStorage(generation.Config{CredentialRef: "aihubkit:image"}),
		a.referenceStorage(generation.Config{CredentialRef: "aihubkit:video"}),
		a.referenceStorage(generation.Config{BaseURL: c.origin + "/v1"}),
		a.referenceStorage(generation.Config{BaseURL: "https://independent.invalid"}),
	} {
		if storage == nil {
			t.Fatal("selected platform reference storage is unavailable")
		}
		if _, err := storage.Upload(context.Background(), "other-workspace", resource.ID); err == nil {
			t.Fatal("platform upload bypassed resource ownership")
		}
		link, err := storage.Upload(context.Background(), "local", resource.ID)
		if err != nil || link != "https://8.8.8.8/reference.png" {
			t.Fatalf("platform reference upload failed: %v", err)
		}
		validator := storage.(*accountStorage)
		if err := validator.ValidateReferences(context.Background()); err != nil {
			t.Fatal(err)
		}
		validator.expiresAt = time.Now().Add(time.Second)
		if validator.ValidateReferences(context.Background()) == nil {
			t.Fatal("platform upload bypassed reference expiration validation")
		}
	}
	if issued != 4 || uploaded != 4 {
		t.Fatalf("expected one upload per owned reference: tickets=%d uploads=%d", issued, uploaded)
	}
}

type uploadPreferenceResources struct {
	generation.ResourcePort
	resource generation.ResourceInfo
}

func (r uploadPreferenceResources) LocalMode() bool { return true }
func (r uploadPreferenceResources) Lookup(_, _ string) (generation.ResourceInfo, error) {
	return r.resource, nil
}

func TestDisabledPlatformUploadsUseNativeStorageWithoutFallback(t *testing.T) {
	t.Setenv("CANVAS_ALLOW_PRIVATE_UPSTREAMS", "true")
	var content bytes.Buffer
	if err := png.Encode(&content, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	var uploads, platformRequests atomic.Int32
	var reject atomic.Bool
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			uploads.Add(1)
			if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=own-access/") {
				t.Error("native upload did not use the configured storage credential")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(body, content.Bytes()) {
				t.Error("native upload changed reference content")
			}
			if reject.Load() {
				w.WriteHeader(http.StatusForbidden)
			}
		} else if r.Method == "GET" {
			if r.Header.Get("Authorization") != "" {
				t.Error("public read check leaked credentials")
			}
			_, _ = w.Write(content.Bytes())
		} else {
			t.Error("unexpected storage method")
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer storage.Close()
	c := newClient()
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		platformRequests.Add(1)
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	a := &account{client: c, token: "management", dir: t.TempDir()}
	runtime, err := bootstrap.Open(context.Background(), bootstrap.Config{
		Profile: bootstrap.ProfileDesktop, DataDir: a.dir, AutoMigrate: true, ConfigureService: a.configure,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	resource, err := a.service.UploadResourceFile("local", "reference.png", int64(content.Len()), "image", 1, 1, 0, bytes.NewReader(content.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	config := referencestorage.Config{Enabled: true, Endpoint: storage.URL, PublicBaseURL: storage.URL, Bucket: "own-bucket", Region: "auto", AccessKeyID: "own-access", SecretAccessKey: "own-secret"}
	if _, err := referencestorage.Save(a.dir, config); err != nil {
		t.Fatal(err)
	}
	ctx := generation.WithRuntime(context.Background(), generation.Runtime{
		Resources:              uploadPreferenceResources{resource: generation.ResourceInfo{ID: resource.ID, Status: "ready", Provider: "local", MimeType: "image/png", Size: resource.Size}},
		SelectReferenceStorage: a.referenceStorage,
		ReferenceStorage:       generation.NewReferenceStoragePort(a.service.IsLocalMode, a.dir, a.service.OpenResource),
	})
	a.uploadToAIHubkit.Store(false)
	for _, channel := range []generation.Config{{CredentialRef: "aihubkit:image"}, {CredentialRef: "aihubkit:video"}, {BaseURL: c.origin + "/v1"}, {BaseURL: "https://independent.invalid"}} {
		input := generation.Input{Config: channel, ReferenceImages: []generation.Media{{StorageKey: "resource:" + resource.ID}}}
		if err := generation.HydrateMedia(ctx, "local", &input, generation.MediaHydrationPolicy{RequireURL: true}); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(input.ReferenceImages[0].URL, storage.URL+"/beeftv/references/") {
			t.Fatal("generation did not use the configured storage URL")
		}
	}
	if uploads.Load() != 4 {
		t.Fatalf("native uploads=%d, want 4", uploads.Load())
	}
	reject.Store(true)
	input := generation.Input{Config: generation.Config{CredentialRef: "aihubkit:video"}, ReferenceImages: []generation.Media{{StorageKey: "resource:" + resource.ID}}}
	if err := generation.HydrateMedia(ctx, "local", &input, generation.MediaHydrationPolicy{RequireURL: true}); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("native storage rejection was hidden: %v", err)
	}
	config.Enabled = false
	if _, err := referencestorage.Save(a.dir, config); err != nil {
		t.Fatal(err)
	}
	if err := generation.HydrateMedia(ctx, "local", &input, generation.MediaHydrationPolicy{RequireURL: true}); err == nil {
		t.Fatal("disabled storage silently uploaded through the platform")
	}
	if platformRequests.Load() != 0 || uploads.Load() != 5 {
		t.Fatalf("unexpected upload fallback: platform=%d native=%d", platformRequests.Load(), uploads.Load())
	}
}
