package app

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type SystemSettings struct {
	PublicURL     string          `json:"publicUrl"`
	PasswordHash  string          `json:"passwordHash"`
	GoogleEnabled bool            `json:"googleEnabled"`
	GoogleID      string          `json:"googleClientId"`
	GoogleSecret  string          `json:"googleClientSecret"`
	AllowedEmails map[string]bool `json:"allowedEmails"`
}

func validateSystem(s SystemSettings) error {
	u, e := url.Parse(s.PublicURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("站点地址必须为不含路径的 HTTP(S) 地址")
	}
	if _, e := bcrypt.Cost([]byte(s.PasswordHash)); e != nil {
		return fmt.Errorf("首次启动需要设置 GATESYNC_ACCESS_PASSWORD（12–72 字节），之后可在系统设置中修改")
	}
	if s.GoogleEnabled && (s.GoogleID == "" || s.GoogleSecret == "" || len(s.AllowedEmails) == 0) {
		return fmt.Errorf("启用 Google 登录需配置 Client ID、Secret 和邮箱白名单")
	}
	return nil
}

func (s *Store) loadSystem() (SystemSettings, error) {
	var settings SystemSettings
	var encoded string
	if e := s.getMeta("system", &encoded); e != nil {
		return settings, e
	}
	encrypted, e := base64.StdEncoding.DecodeString(encoded)
	n := s.aead.NonceSize()
	if e != nil || len(encrypted) < n {
		return settings, fmt.Errorf("系统设置密文损坏")
	}
	raw, e := s.aead.Open(nil, encrypted[:n], encrypted[n:], []byte("system-settings"))
	if e != nil {
		return settings, fmt.Errorf("系统设置无法解密，请恢复原主密钥")
	}
	e = json.Unmarshal(raw, &settings)
	return settings, e
}

func (s *Store) saveSystem(settings SystemSettings, revoke bool) error {
	raw, e := json.Marshal(settings)
	if e != nil {
		return e
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	encrypted := s.aead.Seal(nonce, nonce, raw, []byte("system-settings"))
	encoded, _ := json.Marshal(base64.StdEncoding.EncodeToString(encrypted))
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO meta(key,value) VALUES('system',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(encoded)); e != nil {
		return e
	}
	// 设置与会话撤销一起提交，防止修改登录配置后旧会话继续有效。
	if revoke {
		if _, e = tx.Exec("DELETE FROM sessions"); e != nil {
			return e
		}
	}
	return tx.Commit()
}

func (a *App) initSystem(c Config) error {
	settings, e := a.store.loadSystem()
	if e == sql.ErrNoRows {
		if c.PasswordHash == "" && c.InitialPassword != "" {
			if len(c.InitialPassword) < 12 || len(c.InitialPassword) > 72 {
				return fmt.Errorf("首次初始化密码必须为 12–72 字节")
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(c.InitialPassword), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			c.PasswordHash = string(hash)
		}
		settings = SystemSettings{PublicURL: c.PublicURL, PasswordHash: c.PasswordHash, GoogleEnabled: c.GoogleID != "", GoogleID: c.GoogleID, GoogleSecret: c.GoogleSecret, AllowedEmails: c.AllowedEmails}
		if e = validateSystem(settings); e != nil {
			return e
		}
		if e = a.store.saveSystem(settings, false); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	if e = validateSystem(settings); e != nil {
		return e
	}
	a.system = settings
	return nil
}

func (a *App) systemSettings() SystemSettings {
	a.systemMu.RLock()
	defer a.systemMu.RUnlock()
	return a.system
}

func (a *App) getSystem(w http.ResponseWriter, r *http.Request) {
	s := a.systemSettings()
	emails := []string{}
	for email := range s.AllowedEmails {
		emails = append(emails, email)
	}
	slices.Sort(emails)
	jsonResponse(w, 200, map[string]any{"publicUrl": s.PublicURL, "googleEnabled": s.GoogleEnabled, "googleClientId": s.GoogleID, "hasGoogleSecret": s.GoogleSecret != "", "googleAllowedEmails": emails})
}

func (a *App) updateSystem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PublicURL       string   `json:"publicUrl"`
		GoogleEnabled   bool     `json:"googleEnabled"`
		GoogleID        string   `json:"googleClientId"`
		GoogleSecret    string   `json:"googleClientSecret"`
		AllowedEmails   []string `json:"googleAllowedEmails"`
		CurrentPassword string   `json:"currentPassword"`
		NewPassword     string   `json:"newPassword"`
	}
	if !decode(w, r, &input) {
		return
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	s := a.systemSettings()
	s.PublicURL = strings.TrimRight(strings.TrimSpace(input.PublicURL), "/")
	s.GoogleEnabled = input.GoogleEnabled
	s.GoogleID = strings.TrimSpace(input.GoogleID)
	if len(input.GoogleSecret) > 4096 || len(input.GoogleID) > 1024 || len(input.AllowedEmails) > 100 {
		fail(w, 400, "Google 配置内容过长")
		return
	}
	if input.GoogleSecret != "" {
		s.GoogleSecret = strings.TrimSpace(input.GoogleSecret)
	}
	s.AllowedEmails = map[string]bool{}
	for _, email := range input.AllowedEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		if !strings.Contains(email, "@") || strings.ContainsAny(email, "* \t\r\n") {
			fail(w, 400, "请输入完整的白名单邮箱，不支持通配符")
			return
		}
		s.AllowedEmails[email] = true
	}
	if input.NewPassword != "" {
		if len(input.CurrentPassword) > 72 || bcrypt.CompareHashAndPassword([]byte(s.PasswordHash), []byte(input.CurrentPassword)) != nil {
			fail(w, 400, "当前访问密码不正确")
			return
		}
		if len(input.NewPassword) < 12 || len(input.NewPassword) > 72 {
			fail(w, 400, "新密码必须为 12–72 字节")
			return
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if e != nil {
			fail(w, 500, "密码处理失败")
			return
		}
		s.PasswordHash = string(hash)
	}
	if e := validateSystem(s); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if e := a.store.saveSystem(s, true); e != nil {
		fail(w, 500, "保存系统设置失败")
		return
	}
	a.systemMu.Lock()
	a.system = s
	a.systemMu.Unlock()
	a.oauthMu.Lock()
	clear(a.attempts)
	a.oauthMu.Unlock()
	http.SetCookie(w, a.cookie("gatesync_session", "", -1))
	_ = a.store.addLog(LogEntry{Kind: "settings", Trigger: "manual", Object: "系统设置", Detail: "更新系统及登录设置，已撤销登录会话", Success: true})
	jsonResponse(w, 200, map[string]bool{"ok": true, "reauth": true})
}
