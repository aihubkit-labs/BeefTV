package aihubkit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Replace the file only after the complete new value is written successfully.
func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".aihubkit-setting-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (a *account) loadUploadPreference() error {
	data, err := os.ReadFile(filepath.Join(a.dir, "aihubkit-preferences.json"))
	if errors.Is(err, os.ErrNotExist) {
		return a.saveUploadPreference(true)
	}
	if err != nil {
		return errors.New("无法读取素材上传设置，请检查本地数据目录")
	}
	var saved struct {
		UploadToAIHubkit *bool `json:"uploadToAIHubkit"`
	}
	if json.Unmarshal(data, &saved) != nil || saved.UploadToAIHubkit == nil {
		return errors.New("素材上传设置损坏，请检查本地数据文件")
	}
	a.uploadToAIHubkit.Store(*saved.UploadToAIHubkit)
	return nil
}

func (a *account) saveUploadPreference(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, err := json.Marshal(map[string]bool{"uploadToAIHubkit": enabled})
	if err == nil {
		err = writePrivateFile(filepath.Join(a.dir, "aihubkit-preferences.json"), data)
	}
	if err != nil {
		return errors.New("无法保存素材上传设置，请检查本地数据目录")
	}
	a.uploadToAIHubkit.Store(enabled)
	return nil
}
