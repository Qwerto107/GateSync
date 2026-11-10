package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testFeishuWebhook = "https://open.feishu.cn/open-apis/bot/v2/hook/00000000-0000-0000-0000-000000000001"

type fakeNotificationSender struct {
	events []notificationEvent
	fail   bool
}

func (s *fakeNotificationSender) Send(_ context.Context, _ notificationChannelConfig, e notificationEvent) error {
	s.events = append(s.events, e)
	if s.fail {
		return errors.New("模拟发送失败")
	}
	return nil
}

func notificationFixture(t *testing.T, a *App, events ...string) notificationChannelConfig {
	t.Helper()
	c, e := a.notificationConfig(notificationInput{Name: "测试飞书群", Type: "feishu", Enabled: true, Events: events, Webhook: testFeishuWebhook, SignSecret: "test-sign-secret"})
	if e != nil {
		t.Fatal(e)
	}
	if e := a.store.saveNotificationChannel(c); e != nil {
		t.Fatal(e)
	}
	return c
}

func notificationJobCount(t *testing.T, a *App) int {
	t.Helper()
	var count int
	if e := a.store.db.QueryRow("SELECT count(*) FROM notification_jobs").Scan(&count); e != nil {
		t.Fatal(e)
	}
	return count
}

func TestNotificationChannelsEncryptAndPreserveSecrets(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	input := notificationInput{Name: "运维飞书群", Type: "feishu", Enabled: true, Events: []string{"ip_changed"}, Webhook: testFeishuWebhook, SignSecret: "never-echo-this-secret"}
	w := request(t, a, "POST", "/api/notification-channels", input, cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "never-echo-this-secret") || strings.Contains(w.Body.String(), testFeishuWebhook) {
		t.Fatal(w.Code, w.Body.String())
	}
	var channel notificationChannel
	if e := json.Unmarshal(w.Body.Bytes(), &channel); e != nil || !channel.HasSignSecret || !channel.HasWebhook {
		t.Fatal(e, channel)
	}
	var raw []byte
	if e := a.store.db.QueryRow("SELECT data FROM notification_channels WHERE id=?", channel.ID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "never-echo-this-secret") || strings.Contains(string(raw), testFeishuWebhook) {
		t.Fatal("通知凭证以明文保存")
	}
	w = request(t, a, "GET", "/api/notification-channels", nil, cookie)
	if w.Code != 200 || strings.Contains(w.Body.String(), "never-echo-this-secret") || strings.Contains(w.Body.String(), testFeishuWebhook) {
		t.Fatal("读取渠道回显了秘密", w.Body.String())
	}
	input.ID, input.Webhook, input.SignSecret = channel.ID, "", ""
	w = request(t, a, "PUT", "/api/notification-channels/"+channel.ID, input, cookie)
	c, e := a.store.notificationChannel(channel.ID)
	if w.Code != 200 || e != nil || c.SignSecret != "never-echo-this-secret" || c.Webhook != testFeishuWebhook {
		t.Fatal("留空未保留原凭证", e, w.Body.String())
	}
	input.ClearSignSecret = true
	w = request(t, a, "PUT", "/api/notification-channels/"+channel.ID, input, cookie)
	c, e = a.store.notificationChannel(channel.ID)
	if w.Code != 200 || e != nil || c.SignSecret != "" || c.Channel.HasSignSecret {
		t.Fatal("无法明确清除签名密钥", e)
	}
	for _, path := range []string{"/api/notification-channels", "/api/notification-channels/test", "/api/notification-channels/preview"} {
		method := "POST"
		if path == "/api/notification-channels" {
			method = "GET"
		}
		if w := request(t, a, method, path, nil, nil); w.Code != 401 {
			t.Fatal("通知接口未要求登录", path, w.Code)
		}
	}
	config := a.cfg
	if e := a.Close(); e != nil {
		t.Fatal(e)
	}
	restarted, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	if c, e := restarted.store.notificationChannel(channel.ID); e != nil || c.Webhook != testFeishuWebhook || c.SignSecret != "" {
		t.Fatal("重启未保留渠道配置", e)
	}
}

func TestNotificationValidationPreviewAndExplicitTest(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	fake := &fakeNotificationSender{}
	a.notificationSenders["feishu"] = fake
	input := notificationInput{Name: "测试群", Type: "feishu", Enabled: true, Events: []string{"ip_changed"}, Webhook: testFeishuWebhook, Event: "ip_changed"}
	for _, raw := range []string{"http://open.feishu.cn/open-apis/bot/v2/hook/00000000-0000-0000-0000-000000000001", "https://127.0.0.1/hook", testFeishuWebhook + "?token=secret", strings.Replace(testFeishuWebhook, "open.feishu.cn", "open.feishu.cn.evil.example", 1), strings.Replace(testFeishuWebhook, "open.feishu.cn", "open.feishu.cn@127.0.0.1", 1)} {
		bad := input
		bad.Webhook = raw
		if w := request(t, a, "POST", "/api/notification-channels/test", bad, cookie); w.Code != 400 {
			t.Fatal("接受无效机器人地址", raw, w.Code)
		}
	}
	for _, raw := range []string{`{}`, `[]`, `{"schema":"2.0","body":{"elements":[{"tag":"markdown","content":"{{unknown}}"}]}}`, strings.Repeat(" ", 16*1024+1)} {
		bad := input
		bad.Templates = map[string]string{"ip_changed": raw}
		if w := request(t, a, "POST", "/api/notification-channels", bad, cookie); w.Code != 400 {
			t.Fatal("接受无效模板", w.Code)
		}
	}
	w := request(t, a, "POST", "/api/notification-channels/preview", map[string]any{"event": "ip_changed"}, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "8.8.8.8") || len(fake.events) != 0 || notificationJobCount(t, a) != 0 {
		t.Fatal("预览发送了消息或缺少样例", w.Code)
	}
	w = request(t, a, "POST", "/api/notification-channels/test", input, cookie)
	if w.Code != 200 || len(fake.events) != 1 || !fake.events[0].Test || notificationJobCount(t, a) != 0 {
		t.Fatal("显式测试发送错误", w.Code, len(fake.events))
	}
	var count int
	if e := a.store.db.QueryRow("SELECT count(*) FROM notification_channels").Scan(&count); e != nil || count != 0 {
		t.Fatal("测试擅自保存了表单", count, e)
	}
}

func TestNotificationTemplateEscapingAndTimezone(t *testing.T) {
	event := sampleNotification("sync_failed")
	event.Time, event.Timezone, event.TargetName = "2026-11-10T00:00:00Z", "Asia/Singapore", "\"目标\" <at id=all></at> *test* {{old_ip}}"
	card, e := renderNotificationCard("", event)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(card)
	var decoded map[string]any
	if e := json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal("事件数据破坏了 JSON", e)
	}
	content := notificationCardText(card)
	if strings.Contains(content, "<at id=all>") || !strings.Contains(content, "&lt;at") || !strings.Contains(content, "2026-11-10 08:00:00") || !strings.Contains(content, "{{old\\_ip}}") {
		t.Fatal("模板未安全转义或发生二次插值", content)
	}
	for _, kind := range []string{"ip_changed", "sync_failed"} {
		if _, e := renderNotificationCard("", sampleNotification(kind)); e != nil {
			t.Fatal("默认卡片无效", kind, e)
		}
	}
}

func notificationCardText(card map[string]any) string {
	parts := []string{}
	for _, element := range card["body"].(map[string]any)["elements"].([]any) {
		if text, ok := element.(map[string]any)["content"].(string); ok {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

type notificationRoundTripper func(*http.Request) (*http.Response, error)

func (f notificationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFeishuCardSignatureAndSafeErrors(t *testing.T) {
	if feishuSignature("secret", "1700000000") != "fiWS2+gh28DOydAv7hzONH/mDn9+b1Y4Y5ivXWXy8vA=" {
		t.Fatal("飞书签名与已知向量不一致")
	}
	c := notificationChannelConfig{Channel: notificationChannel{Type: "feishu", Templates: map[string]string{}}, Webhook: testFeishuWebhook, SignSecret: "secret"}
	response := `{"code":0}`
	status := 200
	sender := feishuSender{client: &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != testFeishuWebhook || r.Method != "POST" {
			t.Fatal("请求地址或方法错误")
		}
		var payload map[string]any
		if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
			t.Fatal(e)
		}
		if payload["msg_type"] != "interactive" || payload["sign"] != feishuSignature("secret", payload["timestamp"].(string)) || payload["card"].(map[string]any)["schema"] != "2.0" {
			t.Fatal("未发送签名卡片", payload)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
	})}}
	if e := sender.Send(context.Background(), c, sampleNotification("ip_changed")); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"code":19021,"msg":"` + testFeishuWebhook + ` secret"}`, `{"StatusCode":19024}`, `{}`, `invalid`} {
		response = raw
		e := sender.Send(context.Background(), c, sampleNotification("ip_changed"))
		if e == nil || strings.Contains(e.Error(), testFeishuWebhook) || strings.Contains(e.Error(), "secret") {
			t.Fatal("未拒绝错误响应或泄露秘密", e)
		}
	}
	status, response = 302, `{"code":0}`
	if e := sender.Send(context.Background(), c, sampleNotification("ip_changed")); e == nil {
		t.Fatal("把跳转当作发送成功")
	}
}

func TestNotificationQueueRetryRestartDisableAndDelete(t *testing.T) {
	a, _ := testApp(t)
	c := notificationFixture(t, a, "ip_changed")
	fake := &fakeNotificationSender{fail: true}
	a.notificationSenders["feishu"] = fake
	event := notificationEvent{ID: "same-event", Kind: "ip_changed", OldIP: "8.8.8.8", NewIP: "9.9.9.9", Trigger: "auto"}
	a.enqueueNotification(event)
	a.enqueueNotification(event)
	if notificationJobCount(t, a) != 1 || len(fake.events) != 0 {
		t.Fatal("重复入队或同步调用了网络")
	}
	at := time.Now().Add(time.Second)
	if e := a.deliverNotifications(context.Background(), at); e != nil || len(fake.events) != 1 {
		t.Fatal(e)
	}
	if e := a.deliverNotifications(context.Background(), at.Add(time.Second)); e != nil || len(fake.events) != 1 {
		t.Fatal("重试未遵守间隔", e)
	}
	config := a.cfg
	if e := a.Close(); e != nil {
		t.Fatal(e)
	}
	restarted, e := New(config)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	restarted.notificationSenders["feishu"] = fake
	if e := restarted.deliverNotifications(context.Background(), at.Add(31*time.Second)); e != nil || len(fake.events) != 2 {
		t.Fatal("重启丢失任务或重试计数", e)
	}
	if e := restarted.deliverNotifications(context.Background(), at.Add(3*time.Minute)); e != nil || len(fake.events) != 3 || notificationJobCount(t, restarted) != 0 {
		t.Fatal("未在三次失败后结束", e)
	}
	fake.fail = false
	event.ID = "disabled-event"
	restarted.enqueueNotification(event)
	c.Channel.Enabled = false
	if e := restarted.store.saveNotificationChannel(c); e != nil {
		t.Fatal(e)
	}
	if e := restarted.deliverNotifications(context.Background(), at.Add(4*time.Minute)); e != nil || len(fake.events) != 3 || notificationJobCount(t, restarted) != 0 {
		t.Fatal("停用后仍发送", e)
	}
	c.Channel.Enabled = true
	if e := restarted.store.saveNotificationChannel(c); e != nil {
		t.Fatal(e)
	}
	event.ID = "successful-event"
	restarted.enqueueNotification(event)
	if e := restarted.deliverNotifications(context.Background(), at.Add(5*time.Minute)); e != nil || len(fake.events) != 4 || notificationJobCount(t, restarted) != 0 {
		t.Fatal("成功后未移除任务", e)
	}
	event.ID = "deleted-event"
	restarted.enqueueNotification(event)
	cookie := loginCookie(t, restarted)
	if w := request(t, restarted, "DELETE", "/api/notification-channels/"+c.Channel.ID, nil, cookie); w.Code != 200 || notificationJobCount(t, restarted) != 0 {
		t.Fatal("删除未清理待发送任务", w.Code)
	}
}

func TestNotificationEventsOnlyOnChangedIPAndFailedSync(t *testing.T) {
	a, cloud := testApp(t)
	notificationFixture(t, a, "ip_changed", "sync_failed")
	old := IPState{}
	for i, ip := range []string{"8.8.8.8", "8.8.8.8", "9.9.9.9", "9.9.9.9"} {
		var e error
		old, e = a.saveDetectedIP(context.Background(), old, IPState{IP: ip, CheckedAt: now()}, "manual")
		if e != nil {
			t.Fatal(e)
		}
		want := 0
		if i >= 2 {
			want = 1
		}
		if notificationJobCount(t, a) != want {
			t.Fatal("首次、未变化或重复检测发生额外通知", i)
		}
	}
	target := sourceTarget(t, a)
	cloud.failAdd = true
	if _, e := a.syncTarget(context.Background(), target, "9.9.9.9", "auto", false); e == nil {
		t.Fatal("未触发失败场景")
	}
	if notificationJobCount(t, a) != 2 {
		t.Fatal("同步失败未入队")
	}
	cloud.failAdd = false
	if _, e := a.syncTarget(context.Background(), target, "9.9.9.9", "manual", false); e != nil {
		t.Fatal(e)
	}
	if notificationJobCount(t, a) != 2 {
		t.Fatal("成功同步生成了多余通知")
	}
	setQuerySettings(t, a, true, true)
	if _, e := a.detectIP(context.Background(), "auto"); e == nil {
		t.Fatal("未触发检测失败")
	}
	if notificationJobCount(t, a) != 2 {
		t.Fatal("IP 检测失败生成变更通知")
	}
	var n int
	if e := a.store.db.QueryRow("SELECT count(*) FROM targets").Scan(&n); e != nil || n != 1 {
		t.Fatal(fmt.Sprint(e))
	}
}
