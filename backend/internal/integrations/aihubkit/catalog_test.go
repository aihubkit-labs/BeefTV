package aihubkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/workspace"
)

func TestDefaultModelSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rule   defaultModelRule
		models []CatalogModel
		want   string
	}{
		{"numeric GPT version", defaultModelRules[0], []CatalogModel{{ID: "gpt-5.9"}, {ID: "gpt-image-99"}, {ID: "gpt-5.10-mini"}, {ID: "gpt-5.10"}, {ID: "gpt-4o"}}, "gpt-5.10"},
		{"newer major", defaultModelRules[0], []CatalogModel{{ID: "gpt-5.99"}, {ID: "gpt-6"}}, "gpt-6"},
		{"no GPT", defaultModelRules[0], []CatalogModel{{ID: "alternative"}}, "alternative"},
		{"preferred image", defaultModelRules[1], []CatalogModel{{ID: "other"}, {ID: "gpt-image-2-w"}}, "gpt-image-2-w"},
		{"preferred video", defaultModelRules[2], []CatalogModel{{ID: "other"}, {ID: "sd-mini-z"}}, "sd-mini-z"},
		{"no preferred video", defaultModelRules[2], []CatalogModel{{ID: "alternative"}}, "alternative"},
		{"empty catalog", defaultModelRules[1], []CatalogModel{{ID: " "}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := chooseDefaultModel(tc.rule, tc.models); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	seen := map[string]bool{}
	for range 128 {
		seen[chooseDefaultModel(defaultModelRules[2], []CatalogModel{{ID: "a"}, {ID: "b"}})] = true
	}
	if !reflect.DeepEqual(seen, map[string]bool{"a": true, "b": true}) {
		t.Fatal("fallback must randomly select from the requested group")
	}
}

func TestDefaultModelsPersistPerGroupAndDoNotOverwriteLaterChoices(t *testing.T) {
	groups := map[string]Group{"default-企业版": {}, "gpt_image_web": {}, "video_官key": {}}
	catalogs := map[string][]CatalogModel{
		"default-企业版":   {{ID: "gpt-5.9"}, {ID: "gpt-5.10", Endpoints: []string{"openai-response"}}},
		"gpt_image_web": {{ID: "gpt-image-2-w"}},
		"video_官key":    {{ID: "sd-mini-z", Endpoints: []string{"openai"}}},
	}
	keys := map[string]string{}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/token/":
			var input struct {
				Group string `json:"group"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if _, ok := groups[input.Group]; !ok {
				t.Error("unauthorized group")
			}
			key := "key-" + string(rune('a'+len(keys)))
			keys["sk-"+key] = input.Group
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]string{"key": key}})
		case "/v1/models":
			group := keys[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
			if group == "" {
				t.Error("missing group-specific credential")
			}
			json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": catalogs[group]})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	a := &account{dir: t.TempDir(), client: &client{origin: server.URL, http: server.Client()}, user: User{ID: 7}, token: "management", groups: groups, keys: map[string]string{}}
	store, err := workspace.NewProviderConfig(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.seedModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	body, err := store.ReadLocalModelConfig()
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err = json.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	channels := []any{}
	for _, raw := range config["channels"].([]any) {
		if strings.HasPrefix(stringConfig(raw.(map[string]any)["credentialRef"]), credentialPrefix) {
			channels = append(channels, raw)
		}
	}
	if len(channels) != 3 || len(a.modelImportWarnings) != 0 {
		t.Fatal("three defaults should be imported")
	}
	for i, model := range []string{"gpt-5.10", "gpt-image-2-w", "sd-mini-z"} {
		rule := defaultModelRules[i]
		channel := channels[i].(map[string]any)
		profile := channel["modelProfiles"].([]any)[0].(map[string]any)
		if config[rule.capability+"Model"] != "aihubkit-"+rule.group+"::"+model || channel["credentialRef"] != "aihubkit:"+rule.group || channel["interfaceType"] != nil || profile["protocol"] != rule.protocol || profile["capability"] != rule.capability || profile["model"] != model {
			t.Fatalf("invalid default: %#v", channel)
		}
	}
	config["videoModel"] = "user-selected::model"
	config["channels"] = []any{}
	updated, _ := json.Marshal(config)
	if err = store.SaveLocalModelConfig(updated); err != nil {
		t.Fatal(err)
	}
	updated, _ = store.ReadLocalModelConfig()
	before := requests
	if err = a.seedModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := store.ReadLocalModelConfig()
	if requests != before || string(after) != string(updated) {
		t.Fatal("repeat login overwrote saved choices or fetched catalogs")
	}
	// A subsequent explicit deletion must not bring onboarding defaults back.
	delete(config, "videoModel")
	delete(config, "textModel")
	delete(config, "imageModel")
	updated, _ = json.Marshal(config)
	if err = store.SaveLocalModelConfig(updated); err != nil {
		t.Fatal(err)
	}
	updated, _ = store.ReadLocalModelConfig()
	if err = a.seedModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ = store.ReadLocalModelConfig()
	if requests != before || string(after) != string(updated) {
		t.Fatal("deleted defaults were reimported")
	}
}

func TestMissingOrEmptyDefaultGroupsAreSkippedWithWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatal("unexpected remote request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer server.Close()
	a := &account{dir: t.TempDir(), client: &client{origin: server.URL, http: server.Client()}, groups: map[string]Group{"gpt_image_web": {}}, keys: map[string]string{"gpt_image_web": "test-key"}}
	store, err := workspace.NewProviderConfig(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.seedModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(a.modelImportWarnings) != 3 || !strings.Contains(a.modelImportWarnings[0], "default-企业版") || !strings.Contains(a.modelImportWarnings[1], "没有可用模型") || !strings.Contains(a.modelImportWarnings[2], "video_官key") {
		t.Fatalf("missing skip reasons: %v", a.modelImportWarnings)
	}
	body, _ := store.ReadLocalModelConfig()
	if strings.Contains(string(body), `"textModel"`) || strings.Contains(string(body), `"imageModel"`) || strings.Contains(string(body), `"videoModel"`) {
		t.Fatal("unavailable models became defaults")
	}
}

func TestDefaultModelsInitializePreviouslySavedWorkspaces(t *testing.T) {
	for _, tc := range []struct {
		name, saved string
	}{
		{"empty config", `{"channels":[]}`},
		{"native default", `{"textModel":"custom::chosen","audioModel":"custom::voice","temperature":0.3,"channels":[{"id":"custom","name":"Manual","enabled":true,"apiKey":"synthetic-key","models":["chosen","voice"],"modelProfiles":[{"model":"chosen","capability":"text","protocol":"openai-response"},{"model":"voice","capability":"audio","protocol":"openai-audio"}]}]}`},
		{"existing group models", `{"channels":[{"id":"aihubkit-video_官key","credentialRef":"aihubkit:video_官key","name":"My videos","enabled":true,"models":["previous-video"],"modelProfiles":[{"model":"previous-video","capability":"video","protocol":"newapi","displayName":"Previous","referenceMediaTransport":{"image":"url"}}]}]}`},
		{"preferred model already imported", `{"imageModel":"aihubkit-gpt_image_web::gpt-image-2-w","channels":[{"id":"aihubkit-gpt_image_web","credentialRef":"aihubkit:gpt_image_web","name":"My images","enabled":true,"models":["gpt-image-2-w"],"modelProfiles":[{"model":"gpt-image-2-w","capability":"image","protocol":"openai-image","displayName":"Preferred","capabilityConfig":{"image":{"sizes":["1024x1024"]}}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models := map[string]string{"default-企业版": "gpt-5.10", "gpt_image_web": "gpt-image-2-w", "video_官key": "sd-mini-z"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				model, ok := models[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
				if r.URL.Path != "/v1/models" || !ok {
					t.Error("unexpected group catalog request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []CatalogModel{{ID: model}}})
			}))
			defer server.Close()
			a := &account{dir: t.TempDir(), client: &client{origin: server.URL, http: server.Client()}, groups: map[string]Group{}, keys: map[string]string{}}
			for group := range models {
				a.groups[group] = Group{}
				a.keys[group] = group
			}
			store, err := workspace.NewProviderConfig(a.dir)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.SaveLocalModelConfig([]byte(tc.saved)); err != nil {
				t.Fatal(err)
			}
			before, _, err := store.LoadEffectiveModelConfig()
			if err != nil {
				t.Fatal(err)
			}
			if err = a.seedModels(context.Background()); err != nil {
				t.Fatal(err)
			}
			after, _, err := store.LoadEffectiveModelConfig()
			if err != nil {
				t.Fatal(err)
			}
			channels := map[string]map[string]any{}
			for _, raw := range after.Config["channels"].([]any) {
				channel := raw.(map[string]any)
				id := stringConfig(channel["id"])
				if channels[id] != nil {
					t.Fatalf("duplicate channel %q", id)
				}
				channels[id] = channel
			}
			for _, rule := range defaultModelRules {
				id, model := "aihubkit-"+rule.group, models[rule.group]
				channel := channels[id]
				if channel == nil || channel["credentialRef"] != credentialPrefix+rule.group {
					t.Fatalf("saved workspace did not import %s", rule.capability)
				}
				count := 0
				for _, value := range channel["models"].([]any) {
					if value == model {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("model %q must appear exactly once, got %d", model, count)
				}
				count = 0
				for _, raw := range channel["modelProfiles"].([]any) {
					profile := raw.(map[string]any)
					if profile["model"] == model && profile["capability"] == rule.capability && profile["protocol"] == rule.protocol {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("missing or duplicate profile for %q", model)
				}
				field := rule.capability + "Model"
				want := id + "::" + model
				if after.Config[field] != want {
					t.Fatalf("%s: got %v, want %s", field, after.Config[field], want)
				}
			}
			for _, raw := range before.Config["channels"].([]any) {
				previous := raw.(map[string]any)
				current := channels[stringConfig(previous["id"])]
				if !strings.HasPrefix(stringConfig(previous["credentialRef"]), credentialPrefix) {
					if !reflect.DeepEqual(previous, current) {
						t.Fatal("independent channel changed")
					}
					continue
				}
				if len(current["models"].([]any)) != 1 || len(current["modelProfiles"].([]any)) != 1 || current["name"] == previous["name"] {
					t.Fatal("managed channel was not rebuilt")
				}
			}
			for _, field := range []string{"audioModel", "temperature"} {
				if after.Config[field] != before.Config[field] {
					t.Fatalf("unrelated config %q changed", field)
				}
			}
		})
	}
}

func TestDefaultModelResetPreparesReplacementBeforeSaving(t *testing.T) {
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"success":false,"message":"catalog unavailable"}`))
			return
		}
		w.Write([]byte(`{"object":"list","data":[{"id":"sd-mini-z"}]}`))
	}))
	defer server.Close()
	a := &account{dir: t.TempDir(), client: &client{origin: server.URL, http: server.Client()}, groups: map[string]Group{"video_官key": {}}, keys: map[string]string{"video_官key": "fixture"}}
	store, err := workspace.NewProviderConfig(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLocalModelConfig([]byte(`{"aihubkitDefaultsInitialized":true,"videoModel":"aihubkit-other::old","audioModel":"aihubkit-other::old","channels":[{"id":"aihubkit-other","credentialRef":"aihubkit:other","models":["old"]},{"id":"custom","apiKey":"synthetic-key","models":["manual"]}]}`)); err != nil {
		t.Fatal(err)
	}
	before, _ := store.ReadLocalModelConfig()
	config, _, _ := store.LoadEffectiveModelConfig()
	if _, err := a.defaultModelConfig(context.Background(), config.Config); err == nil {
		t.Fatal("failed catalog lookup produced a replacement")
	}
	after, _ := store.ReadLocalModelConfig()
	if string(before) != string(after) {
		t.Fatal("failed initialization changed saved models")
	}
	fail = false
	config, _, _ = store.LoadEffectiveModelConfig()
	result, err := a.defaultModelConfig(context.Background(), config.Config)
	if err != nil {
		t.Fatal(err)
	}
	after, _ = store.ReadLocalModelConfig()
	if string(before) != string(after) {
		t.Fatal("preparing the replacement changed saved models")
	}
	if result.Config["videoModel"] != "aihubkit-video_官key::sd-mini-z" || result.Config["audioModel"] != "" || len(result.Warnings) != 2 {
		t.Fatal("reset did not restore defaults or report unavailable groups")
	}
	for _, raw := range result.Config["channels"].([]any) {
		channel := raw.(map[string]any)
		if channel["id"] == "aihubkit-other" {
			t.Fatal("old imported group survived initialization")
		}
		if channel["id"] == "custom" && channel["apiKey"] != "synthetic-key" {
			t.Fatal("independent channel was changed")
		}
	}
}
