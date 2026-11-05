package aihubkit

import (
	"os"
	"path/filepath"
	"testing"

	"infinite-canvas/backend/internal/app"
	"infinite-canvas/backend/internal/integrations/aihubkit/branding"
)

func TestAssistantUsesBoundBrandsGroupKeyAndAPIOrigin(t *testing.T) {
	dir := t.TempDir()
	config := `{"schemaVersion":1,"revision":1,"config":{"textModel":"managed::chat",
	"channels":[{"id":"managed","name":"ZModel · group","enabled":true,"credentialRef":"aihubkit:group",
	"baseUrl":"https://stale.example","apiKey":"","modelProfiles":[{"model":"chat","capability":"text","protocol":"chat-completion"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "local-model-config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	c := newClient()
	a := &account{client: c, groups: map[string]Group{"group": {}}, keys: map[string]string{"group": "synthetic-group-key"}}
	svc := app.NewLocal(nil, dir)
	if err := a.configure(svc); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://api.brand-one.example", "https://api.brand-two.example"} {
		c.brand = branding.New(branding.Brand{ID: "brand", PlatformName: "ZModel", AppName: "ZModelTV", APIOrigin: origin})
		provider, err := svc.ResolveAssistantProvider()
		if err != nil || provider.APIKey != "synthetic-group-key" || provider.BaseURL != origin {
			t.Fatal("assistant did not resolve the current brand's API origin and group key")
		}
	}
	delete(a.keys, "group")
	if _, err := svc.ResolveAssistantProvider(); err == nil {
		t.Fatal("missing group credential was accepted")
	}
}
