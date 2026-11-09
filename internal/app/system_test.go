package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSystemSettingsPersistAndRevokeSessions(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	input := map[string]any{"publicUrl": "http://localhost:13850", "googleEnabled": false, "googleClientId": "test-client", "googleClientSecret": "secret-only-in-ciphertext", "googleAllowedEmails": []string{"admin@example.com"}, "currentPassword": "incorrect", "newPassword": "new-password-2026"}
	w := request(t, a, "PUT", "/api/system", input, cookie)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(t, a, "GET", "/api/state", nil, cookie); w.Code != 200 {
		t.Fatal("无效修改撤销了已有会话")
	}
	input["currentPassword"] = "test-password-2026"
	w = request(t, a, "PUT", "/api/system", input, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(t, a, "GET", "/api/state", nil, cookie); w.Code != 401 {
		t.Fatal("密码修改后旧会话仍有效")
	}
	var raw string
	if e := a.store.db.QueryRow("SELECT value FROM meta WHERE key='system'").Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(raw, "secret-only-in-ciphertext") || strings.Contains(raw, "new-password-2026") {
		t.Fatal("系统秘密以明文保存")
	}
	w = request(t, a, "POST", "/api/auth/login", map[string]any{"password": "new-password-2026", "remember": false}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	nextCookie := w.Result().Cookies()[0]
	w = request(t, a, "GET", "/api/system", nil, nextCookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-only-in-ciphertext") || strings.Contains(w.Body.String(), "passwordHash") {
		t.Fatal("系统读取接口回显了秘密", w.Body.String())
	}
	dir := a.cfg.DataDir
	if e := a.Close(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("GATESYNC_ACCESS_PASSWORD", "old-short")
	t.Setenv("GATESYNC_DATA_DIR", dir)
	config, e := LoadConfig()
	if e != nil {
		t.Fatal(e)
	}
	restarted, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	if restarted.systemSettings().PublicURL != "http://localhost:13850" || restarted.systemSettings().GoogleSecret != "secret-only-in-ciphertext" {
		t.Fatal("重启后被启动默认值覆盖")
	}
	w = request(t, restarted, "POST", "/api/auth/login", map[string]any{"password": "test-password-2026", "remember": false}, nil)
	if w.Code != 401 {
		t.Fatal("重启恢复了旧密码")
	}
}

func TestLighthouseTargetUsesItsOwnAdapter(t *testing.T) {
	a, vpc := testApp(t)
	light := &fakeCloud{version: 1}
	a.lighthouse = func(Credentials, string) (Cloud, error) { return light, nil }
	cookie := loginCookie(t, a)
	w := request(t, a, "POST", "/api/accounts", map[string]any{"resourceType": "lighthouse", "name": "轻量账号", "region": "ap-guangzhou", "secretId": "test-id", "secretKey": "test-key"}, cookie)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var account Account
	_ = json.Unmarshal(w.Body.Bytes(), &account)
	input := map[string]any{"resourceType": "lighthouse", "accountId": account.ID, "region": "ap-guangzhou", "groupId": "lhins-test", "ports": []Port{{"TCP", "22"}}, "ipMode": "fixed", "fixedCidr": "8.8.8.8/32", "autoSync": false}
	w = request(t, a, "POST", "/api/targets", input, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var target Target
	_ = json.Unmarshal(w.Body.Bytes(), &target)
	if target.Status != "synced" || target.ResourceType != "lighthouse" || len(light.rules) != 1 || len(vpc.rules) != 0 {
		t.Fatal("轻量目标使用了错误的适配器", target)
	}
	if _, e := a.syncTarget(context.Background(), target, "", "manual", true); e != nil {
		t.Fatal(e)
	}
	if len(light.rules) != 0 {
		t.Fatal("轻量规则未撤销")
	}
}

func TestCronUsesConfiguredTimezone(t *testing.T) {
	s, e := (Settings{Cron: "0 9 * * *", Timezone: "Asia/Tokyo"}).schedule()
	if e != nil {
		t.Fatal(e)
	}
	next := s.Next(time.Date(2026, 11, 9, 23, 0, 0, 0, time.UTC))
	expected := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Fatal(next, expected)
	}
	if _, e := (Settings{Interval: 5, Timezone: "invalid/timezone"}).schedule(); e == nil {
		t.Fatal("接受了无效时区")
	}
}
