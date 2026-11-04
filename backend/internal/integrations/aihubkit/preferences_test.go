package aihubkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadPreferencePersistsBothChoices(t *testing.T) {
	dir := t.TempDir()
	a := &account{dir: dir}
	if err := a.loadUploadPreference(); err != nil || !a.uploadToAIHubkit.Load() {
		t.Fatal("first login must default to platform uploads", err)
	}
	for _, enabled := range []bool{false, true} {
		if err := a.saveUploadPreference(enabled); err != nil {
			t.Fatal(err)
		}
		reopened := &account{dir: dir}
		if err := reopened.loadUploadPreference(); err != nil || reopened.uploadToAIHubkit.Load() != enabled {
			t.Fatal("saved choice did not survive a new runtime", err)
		}
	}
}

func TestUploadPreferenceRejectsCorruptionAndPreservesLiveChoiceOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aihubkit-preferences.json")
	a := &account{dir: dir}
	for _, value := range []string{`{}`, `{"uploadToAIHubkit":null}`, `{"uploadToAIHubkit":"false"}`, `broken`} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if err := a.loadUploadPreference(); err == nil || a.uploadToAIHubkit.Load() {
			t.Fatal("corrupt preference silently enabled uploads")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.saveUploadPreference(true); err == nil || a.uploadToAIHubkit.Load() {
		t.Fatal("failed save changed the live preference")
	}
}
