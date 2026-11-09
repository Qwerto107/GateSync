package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testApp(t *testing.T) (*App, *fakeCloud) {
	t.Helper()
	t.Setenv("GATESYNC_MASTER_KEY", "")
	hash, e := bcrypt.GenerateFromPassword([]byte("test-password-2026"), bcrypt.MinCost)
	if e != nil {
		t.Fatal(e)
	}
	a, e := New(Config{DataDir: t.TempDir(), PublicURL: "http://localhost:8080", PasswordHash: string(hash), AllowedEmails: map[string]bool{}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	cloud := &fakeCloud{version: 1}
	a.cloud = func(Credentials, string) (Cloud, error) { return cloud, nil }
	return a, cloud
}
func request(t *testing.T, a *App, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set("X-GateSync-Request", "1")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}
func loginCookie(t *testing.T, a *App) *http.Cookie {
	t.Helper()
	w := request(t, a, "POST", "/api/auth/login", map[string]any{"password": "test-password-2026", "remember": true}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "gatesync_session" {
			if !c.HttpOnly || c.MaxAge != 604800 {
				t.Fatal("会话 Cookie 属性错误")
			}
			return c
		}
	}
	t.Fatal("没有会话 Cookie")
	return nil
}
func TestAuthAndCSRF(t *testing.T) {
	a, _ := testApp(t)
	if w := request(t, a, "GET", "/api/state", nil, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(t, a, "POST", "/api/auth/login", map[string]any{"password": "wrong"}, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie := loginCookie(t, a)
	if w := request(t, a, "GET", "/api/state", nil, cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, mode := range []string{"header", "origin"} {
		r := httptest.NewRequest("POST", "/api/ip/detect", nil)
		r.AddCookie(cookie)
		if mode == "origin" {
			r.Header.Set("X-GateSync-Request", "1")
			r.Header.Set("Origin", "https://evil.example")
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(mode, w.Code)
		}
	}
	if w := request(t, a, "POST", "/api/auth/logout", nil, cookie); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(t, a, "GET", "/api/state", nil, cookie); w.Code != 401 {
		t.Fatal("退出后会话仍有效")
	}
}
func TestAccountTargetAndRevokeFlow(t *testing.T) {
	a, cloud := testApp(t)
	cookie := loginCookie(t, a)
	w := request(t, a, "POST", "/api/accounts", map[string]any{"name": "办公室", "region": "ap-guangzhou", "secretId": "AKID-test-123456", "secretKey": "secret-do-not-expose"}, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var account Account
	_ = json.Unmarshal(w.Body.Bytes(), &account)
	w = request(t, a, "GET", "/api/state", nil, cookie)
	if strings.Contains(w.Body.String(), "secret-do-not-expose") || strings.Contains(w.Body.String(), "AKID-test-123456") {
		t.Fatal("API 泄露凭证")
	}
	var raw []byte
	if e := a.store.db.QueryRow("SELECT credentials FROM accounts WHERE id=?", account.ID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(raw, []byte("secret-do-not-expose")) {
		t.Fatal("凭证未加密")
	}
	input := map[string]any{"accountId": account.ID, "region": "ap-guangzhou", "groupId": "sg-test", "groupName": "办公 SSH", "ports": []Port{{Protocol: "TCP", Value: "22"}}, "ipMode": "fixed", "fixedCidr": "8.8.8.8", "autoSync": true, "note": "测试"}
	w = request(t, a, "POST", "/api/targets", input, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var target Target
	_ = json.Unmarshal(w.Body.Bytes(), &target)
	if target.Status != "synced" || len(cloud.rules) != 1 {
		t.Fatal(w.Body.String(), cloud.rules)
	}
	w = request(t, a, "PATCH", "/api/targets/"+target.ID+"/auto", map[string]bool{"enabled": false}, cookie)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	got, _ := a.store.target(target.ID)
	if got.AutoSync {
		t.Fatal("自动同步未关闭")
	}
	// 动态目标的撤销不需要可用公网 IP；退出旧网络后仍可清理授权。
	got.IPMode = "dynamic"
	if e := a.store.saveTarget(got); e != nil {
		t.Fatal(e)
	}
	w = request(t, a, "DELETE", "/api/accounts/"+account.ID+"?revoke=true", nil, cookie)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(cloud.rules) != 0 {
		t.Fatal("规则未撤销")
	}
	targets, _ := a.store.targets()
	accounts, _ := a.store.accounts()
	if len(targets) != 0 || len(accounts) != 0 {
		t.Fatal("本地数据未删除")
	}
}
func TestDeletePreservesLocalRecordsWhenRevokeFails(t *testing.T) {
	a, cloud := testApp(t)
	c := Credentials{SecretID: "id", SecretKey: "key"}
	account := Account{ID: randomID(), Name: "测试", Region: "ap-guangzhou"}
	if e := a.store.saveAccount(account, &c); e != nil {
		t.Fatal(e)
	}
	target := Target{ID: randomID(), AccountID: account.ID, Region: "ap-guangzhou", GroupID: "sg-test", Ports: []Port{{"TCP", "22"}}, IPMode: "fixed", FixedCIDR: "8.8.8.8/32"}
	if e := a.store.saveTarget(target); e != nil {
		t.Fatal(e)
	}
	if _, e := a.syncTarget(context.Background(), target, "", "manual", false); e != nil {
		t.Fatal(e)
	}
	cloud.failDelete = true
	w := request(t, a, "DELETE", "/api/accounts/"+account.ID+"?revoke=true", nil, loginCookie(t, a))
	if w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := a.store.account(account.ID); e != nil {
		t.Fatal("撤销失败时账号被删除")
	}
	if _, e := a.store.target(target.ID); e != nil {
		t.Fatal("撤销失败时目标被删除")
	}
}
func TestGoogleRejectsUnsolicitedCallback(t *testing.T) {
	a, _ := testApp(t)
	w := request(t, a, "GET", "/api/auth/google/callback?state=forged&code=forged", nil, nil)
	if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "google_denied") {
		t.Fatal(w.Code, w.Header())
	}
}

func TestPartialAccountRevokeDoesNotReopenCompletedTargets(t *testing.T) {
	a, cloud := testApp(t)
	c := Credentials{SecretID: "id", SecretKey: "key"}
	account := Account{ID: randomID(), Name: "测试", Region: "ap-guangzhou"}
	if e := a.store.saveAccount(account, &c); e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	for _, group := range []string{"sg-first", "sg-second"} {
		target := Target{ID: randomID(), AccountID: account.ID, Region: "ap-guangzhou", GroupID: group, Ports: []Port{{"TCP", "22"}}, IPMode: "fixed", FixedCIDR: "8.8.8.8/32", AutoSync: true}
		if e := a.store.saveTarget(target); e != nil {
			t.Fatal(e)
		}
		if _, e := a.syncTarget(context.Background(), target, "", "manual", false); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, target.ID)
	}
	cloud.failSecondDelete = true
	w := request(t, a, "DELETE", "/api/accounts/"+account.ID+"?revoke=true", nil, loginCookie(t, a))
	if w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	first, e := a.store.target(ids[0])
	if e != nil {
		t.Fatal(e)
	}
	if first.AutoSync || first.Status != "revoked" || first.CurrentCIDR != "" {
		t.Fatal("已撤销目标可能被自动同步重新开放", first)
	}
	if _, e = a.store.account(account.ID); e != nil {
		t.Fatal("部分撤销失败时账号丢失")
	}
}
func TestValidationAndCSV(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.1.1", "169.254.169.254", "100.64.0.1", "203.0.113.1", "::1"} {
		if publicIPv4(netip.MustParseAddr(s)) {
			t.Fatal("接受了非公网地址", s)
		}
	}
	if !publicIPv4(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("有效公网地址被拒绝")
	}
	for _, s := range []string{"http://api.ipify.org", "https://user:pass@example.com", "https://example.com:8080"} {
		if validateSource(s) == nil {
			t.Fatal("接受了不安全的检测源", s)
		}
	}
	s, e := normalizeCIDR("8.8.8.123/24")
	if e != nil || s != "8.8.8.0/24" {
		t.Fatal(s, e)
	}
	for _, s := range []string{"0.0.0.0", "0.0.0.0/24", "1.2.3.4/1"} {
		if _, e := normalizeCIDR(s); e == nil {
			t.Fatal("接受了可能被腾讯云扩大为全网的地址", s)
		}
	}
	for _, s := range []string{"=CMD()", " +CMD()", "@SUM(1)", "-2+2"} {
		if !strings.HasPrefix(csvSafe(s), "'") {
			t.Fatal("CSV 公式未转义", s)
		}
	}
	for _, p := range []string{"0", "65536", "100-20", "abc"} {
		target := Target{AccountID: "a", Region: "ap-guangzhou", GroupID: "sg-test", IPMode: "dynamic", Ports: []Port{{"TCP", p}}}
		if validateTarget(&target) == nil {
			t.Fatal("接受了无效端口", p)
		}
	}
	for _, interval := range []int{1, 3, 5, 10, 15, 30, 60, 10080} {
		schedule, e := (Settings{Interval: interval}).schedule()
		start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
		if e != nil || schedule.Next(start).Sub(start) != time.Duration(interval)*time.Minute {
			t.Fatal("间隔调度错误", interval, e)
		}
	}
	for _, interval := range []int{-1, 0, 10081} {
		if _, e := (Settings{Interval: interval}).schedule(); e == nil {
			t.Fatal("接受了无效间隔", interval)
		}
	}
	if _, e := (Settings{Cron: "*/5 * * * *"}).schedule(); e != nil {
		t.Fatal(e)
	}
	if _, e := (Settings{Cron: "@every 1s"}).schedule(); e == nil {
		t.Fatal("接受了秒级任务表达式")
	}
}
func TestStoreFailsWhenMasterKeyMissing(t *testing.T) {
	t.Setenv("GATESYNC_MASTER_KEY", "")
	dir := t.TempDir()
	s, e := openStore(dir, "")
	if e != nil {
		t.Fatal(e)
	}
	s.db.Close()
	if e = os.Remove(dir + "/master.key"); e != nil {
		t.Fatal(e)
	}
	if _, e = openStore(dir, ""); e == nil {
		t.Fatal("丢失主密钥时创建了替代密钥")
	}
	wrongKey := base64.StdEncoding.EncodeToString(make([]byte, 31))
	if _, e = openStore(t.TempDir(), wrongKey); e == nil {
		t.Fatal("接受了错误长度的密钥")
	}
}
