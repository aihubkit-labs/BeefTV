package aihubkit

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"infinite-canvas/backend/internal/bootstrap"
	"infinite-canvas/backend/internal/localcrypto"
)

type rememberedLogin struct {
	Origin   string `json:"origin"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Host) loginFormDir() string {
	return filepath.Join(h.cfg.DataDir, "aihubkit", "login")
}

func (h *Host) saveLoginForm(username, password string, remember bool) error {
	path := filepath.Join(h.loginFormDir(), "credentials.enc")
	if !remember {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("无法清除记住的账号密码，请检查本地数据目录")
		}
		return nil
	}
	data, err := json.Marshal(rememberedLogin{Origin: h.client.origin, Username: username, Password: password})
	if err != nil {
		return errors.New("无法保存账号密码")
	}
	enc, err := localcrypto.Encrypt(h.loginFormDir(), string(data))
	if err == nil {
		err = writePrivateFile(path, []byte(enc))
	}
	if err != nil {
		return errors.New("无法保存账号密码，请检查本地数据目录")
	}
	return nil
}

func (h *Host) loadLoginForm() (*rememberedLogin, error) {
	data, err := os.ReadFile(filepath.Join(h.loginFormDir(), "credentials.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("无法读取记住的账号密码，可手动输入后登录")
	}
	plain, err := localcrypto.Decrypt(h.loginFormDir(), string(data))
	var saved rememberedLogin
	if err != nil || json.Unmarshal([]byte(plain), &saved) != nil || saved.Username == "" || saved.Password == "" {
		return nil, errors.New("无法解密记住的账号密码，可手动输入后登录")
	}
	if saved.Origin != h.client.origin {
		return nil, nil
	}
	return &saved, nil
}

// Autofill is available only to the launch-token-protected desktop UI. It
// never authenticates the account or returns passwords in ordinary sessions.
func (h *Host) loginFormAction(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Profile != bootstrap.ProfileDesktop || h.launchToken == "" || !sameSecret(r.Header.Get("X-Desktop-Token"), h.launchToken) {
		reply(w, 403, nil, "仅桌面应用可读取记住的账号密码", "desktop_token_required")
		return
	}
	if r.Header.Get("X-Aihubkit-Client") != "1" || (r.Method != "POST" && r.Method != "DELETE") {
		reply(w, 400, nil, "请求无效", "invalid_request")
		return
	}
	h.transition.Lock()
	defer h.transition.Unlock()
	h.mu.RLock()
	busy := h.active != nil || h.closed || h.migrating
	h.mu.RUnlock()
	if busy {
		reply(w, 409, nil, "请返回登录页后操作", "workspace_in_use")
		return
	}
	if r.Method == "DELETE" {
		if err := h.saveLoginForm("", "", false); err != nil {
			reply(w, 500, nil, err.Error(), "login_form_failed")
			return
		}
		reply(w, 200, true, "", "")
		return
	}
	saved, err := h.loadLoginForm()
	if err != nil {
		reply(w, 500, nil, err.Error(), "login_form_failed")
		return
	}
	if saved == nil {
		reply(w, 200, nil, "", "")
		return
	}
	reply(w, 200, map[string]string{"username": saved.Username, "password": saved.Password}, "", "")
}
