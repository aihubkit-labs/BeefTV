package app

import (
	"errors"
	"testing"
)

func TestAssistantUsesOptionalCredentialResolver(t *testing.T) {
	service := writeAssistantConfig(t, `{"schemaVersion":1,"revision":1,"config":{
		"textModel":"managed::chat","channels":[{"id":"managed","name":"ZModel · group","enabled":true,
		"credentialRef":"distribution:group","baseUrl":"https://old.example","apiKey":"stale-inline-key",
		"modelProfiles":[{"model":"chat","capability":"text","protocol":"chat-completion"}]}]}}`)
	for _, unavailable := range []bool{false, true} {
		service.SetExtensions(Extensions{ResolveCredential: func(ref string) (string, string, bool, error) {
			if ref != "distribution:group" {
				t.Fatal("credential reference was not forwarded")
			}
			if unavailable {
				return "", "", true, errors.New("credential unavailable")
			}
			return "https://api.example", "managed-secret", true, nil
		}})
		provider, err := service.ResolveAssistantProvider()
		if unavailable {
			var missing *AssistantUnavailableError
			if !errors.As(err, &missing) || missing.Reason != AssistantReasonCredentialMissing || provider.APIKey != "" {
				t.Fatal("managed credential failure must not fall back to an inline key")
			}
		} else if err != nil || provider.APIKey != "managed-secret" || provider.BaseURL != "https://api.example" || provider.ChannelName != "ZModel · group" {
			t.Fatal("assistant did not use the managed channel credential and origin")
		}
	}
}
