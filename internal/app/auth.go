package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type identityKey struct{}

func hashToken(raw string) string { b := sha256.Sum256([]byte(raw)); return hex.EncodeToString(b[:]) }
func (a *App) cookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.systemSettings().PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: maxAge}
}
func (a *App) newSession(w http.ResponseWriter, identity string, remember bool) error {
	raw := randomID() + randomID()
	age := 12 * time.Hour
	maxAge := 0
	if remember {
		age = 7 * 24 * time.Hour
		maxAge = int(age.Seconds())
	}
	_, e := a.store.db.Exec("INSERT INTO sessions(token,identity,expires) VALUES(?,?,?)", hashToken(raw), identity, time.Now().Add(age).Unix())
	if e != nil {
		return e
	}
	_, _ = a.store.db.Exec("DELETE FROM sessions WHERE expires<?", time.Now().Unix())
	http.SetCookie(w, a.cookie("gatesync_session", raw, maxAge))
	return nil
}
func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie("gatesync_session")
		if e != nil {
			fail(w, 401, "请先登录")
			return
		}
		var identity string
		if e = a.store.db.QueryRow("SELECT identity FROM sessions WHERE token=? AND expires>?", hashToken(cookie.Value), time.Now().Unix()).Scan(&identity); e != nil {
			fail(w, 401, "登录已失效，请重新登录")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if !decode(w, r, &input) {
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.rateMu.Lock()
	cutoff := time.Now().Add(-10 * time.Minute)
	// 不信任转发头；反向代理可另外在边缘施加限流。
	for k, ts := range a.loginRates {
		recent := []time.Time{}
		for _, t := range ts {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}
		if len(recent) == 0 {
			delete(a.loginRates, k)
		} else {
			a.loginRates[k] = recent
		}
	}
	blocked := len(a.loginRates[host]) >= 8 || len(a.loginRates) >= 4096
	if !blocked {
		a.loginRates[host] = append(a.loginRates[host], time.Now())
	}
	a.rateMu.Unlock()
	if blocked {
		w.Header().Set("Retry-After", "600")
		fail(w, 429, "登录尝试过于频繁，请 10 分钟后重试")
		return
	}
	select {
	case a.loginSlots <- struct{}{}:
		defer func() { <-a.loginSlots }()
	default:
		fail(w, 429, "登录请求繁忙，请稍后重试")
		return
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if len(input.Password) > 72 || bcrypt.CompareHashAndPassword([]byte(a.systemSettings().PasswordHash), []byte(input.Password)) != nil {
		_ = a.store.addLog(LogEntry{Kind: "login", Trigger: "manual", Object: "访问密码", Detail: "密码验证失败", Success: false})
		fail(w, 401, "访问密码不正确")
		return
	}
	if e := a.newSession(w, "admin", input.Remember); e != nil {
		fail(w, 500, "无法保存登录会话")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "login", Trigger: "manual", Object: "admin", Detail: "通过访问密码登录", Success: true})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("gatesync_session"); e == nil {
		_, _ = a.store.db.Exec("DELETE FROM sessions WHERE token=?", hashToken(c.Value))
	}
	http.SetCookie(w, a.cookie("gatesync_session", "", -1))
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func oauthConfig(s SystemSettings) *oauth2.Config {
	return &oauth2.Config{ClientID: s.GoogleID, ClientSecret: s.GoogleSecret, RedirectURL: s.PublicURL + "/api/auth/google/callback", Endpoint: google.Endpoint, Scopes: []string{oidc.ScopeOpenID, "email", "profile"}}
}
func (a *App) googleStart(w http.ResponseWriter, r *http.Request) {
	settings := a.systemSettings()
	if !settings.GoogleEnabled {
		http.Redirect(w, r, "/?login_error=google_disabled", http.StatusSeeOther)
		return
	}
	key := randomID()
	attempt := oauthAttempt{State: randomID(), Verifier: oauth2.GenerateVerifier(), Nonce: randomID(), Expires: time.Now().Add(10 * time.Minute)}
	a.oauthMu.Lock()
	for k, v := range a.attempts {
		if v.Expires.Before(time.Now()) {
			delete(a.attempts, k)
		}
	}
	if len(a.attempts) >= 128 {
		a.oauthMu.Unlock()
		fail(w, 429, "登录请求过于频繁")
		return
	}
	a.attempts[key] = attempt
	a.oauthMu.Unlock()
	http.SetCookie(w, a.cookie("gatesync_oauth", key, 600))
	http.Redirect(w, r, oauthConfig(settings).AuthCodeURL(attempt.State, oauth2.S256ChallengeOption(attempt.Verifier), oidc.Nonce(attempt.Nonce)), http.StatusFound)
}
func (a *App) googleCallback(w http.ResponseWriter, r *http.Request) {
	denied := func() {
		_ = a.store.addLog(LogEntry{Kind: "login", Trigger: "manual", Object: "Google OAuth", Detail: "Google 登录未完成或账号未授权", Success: false})
		http.Redirect(w, r, "/?login_error=google_denied", http.StatusSeeOther)
	}
	c, e := r.Cookie("gatesync_oauth")
	if e != nil {
		denied()
		return
	}
	http.SetCookie(w, a.cookie("gatesync_oauth", "", -1))
	a.oauthMu.Lock()
	attempt, ok := a.attempts[c.Value]
	delete(a.attempts, c.Value)
	a.oauthMu.Unlock()
	if !ok || time.Now().After(attempt.Expires) || subtle.ConstantTimeCompare([]byte(attempt.State), []byte(r.URL.Query().Get("state"))) != 1 || r.URL.Query().Get("code") == "" {
		denied()
		return
	}
	settings := a.systemSettings()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	token, e := oauthConfig(settings).Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(attempt.Verifier))
	if e != nil {
		denied()
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		denied()
		return
	}
	provider, e := oidc.NewProvider(ctx, "https://accounts.google.com")
	if e != nil {
		denied()
		return
	}
	id, e := provider.Verifier(&oidc.Config{ClientID: settings.GoogleID}).Verify(ctx, raw)
	if e != nil || id.Nonce != attempt.Nonce {
		denied()
		return
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if e = id.Claims(&claims); e != nil || !claims.Verified || !settings.AllowedEmails[strings.ToLower(claims.Email)] {
		denied()
		return
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	current := a.systemSettings()
	if !current.GoogleEnabled || current.GoogleID != settings.GoogleID || current.GoogleSecret != settings.GoogleSecret || current.PublicURL != settings.PublicURL || !current.AllowedEmails[strings.ToLower(claims.Email)] {
		denied()
		return
	}
	if e = a.newSession(w, claims.Email, false); e != nil {
		denied()
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "login", Trigger: "manual", Object: claims.Email, Detail: "通过 Google OAuth 登录", Success: true})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
