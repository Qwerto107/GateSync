package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func queuedNotification(t *testing.T, a *App, channel string) notificationEvent {
	t.Helper()
	var raw string
	if e := a.store.db.QueryRow("SELECT event FROM notification_jobs WHERE channel_id=?", channel).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var event notificationEvent
	if e := json.Unmarshal([]byte(raw), &event); e != nil {
		t.Fatal(e)
	}
	return event
}

func TestNotificationRoundMergesPerSubscriptionAndRefiltersBeforeSend(t *testing.T) {
	a, cloud := testApp(t)
	all := notificationFixture(t, a, "ip_changed", "sync_succeeded", "sync_failed")
	succeeded := notificationFixture(t, a, "sync_succeeded")
	failed := notificationFixture(t, a, "sync_failed")
	ipOnly := notificationFixture(t, a, "ip_changed")
	fake := &fakeNotificationSender{}
	a.notificationSenders["feishu"] = fake
	ctx, finish := a.beginNotificationRound(context.Background())
	if _, e := a.saveDetectedIP(ctx, IPState{IP: "8.8.8.8"}, IPState{IP: "9.9.9.9", CheckedAt: now()}, "auto"); e != nil {
		t.Fatal(e)
	}
	target := sourceTarget(t, a)
	target.Name = "成功目标"
	if _, e := a.syncTarget(ctx, target, "9.9.9.9", "auto", false); e != nil {
		t.Fatal(e)
	}
	cloud.failAdd = true
	target.ID, target.Name = randomID(), "失败目标"
	if _, e := a.syncTarget(ctx, target, "9.9.9.9", "auto", false); e == nil {
		t.Fatal("未触发失败")
	}
	if notificationJobCount(t, a) != 0 || len(fake.events) != 0 {
		t.Fatal("一轮未结束就发送或入队")
	}
	finish()
	if notificationJobCount(t, a) != 4 {
		t.Fatal("未按渠道合并")
	}
	event := queuedNotification(t, a, all.Channel.ID)
	values := event.variables()
	if event.Kind != "sync_summary" || len(event.Items) != 3 || values["success_count"] != "1" || values["failure_count"] != "1" || values["title"] != "规则同步部分失败" || !strings.Contains(values["ip_summary"], "8.8.8.8 → 9.9.9.9") || !strings.Contains(values["summary"], "成功目标") || !strings.Contains(values["summary"], "失败目标") {
		t.Fatal("合并摘要或部分失败标题错误", event, values)
	}
	for channel, kind := range map[string]string{succeeded.Channel.ID: "sync_succeeded", failed.Channel.ID: "sync_failed", ipOnly.Channel.ID: "ip_changed"} {
		event := queuedNotification(t, a, channel)
		if event.Kind != kind || len(event.Items) != 1 {
			t.Fatal("渠道包含未订阅事件", event)
		}
	}
	// 排队后取消失败订阅：发送时重新生成摘要，不能保留旧计数或失败文案。
	all.Channel.Events = []string{"sync_succeeded", "ip_changed"}
	if e := a.store.saveNotificationChannel(all); e != nil {
		t.Fatal(e)
	}
	if e := a.deliverNotifications(context.Background(), time.Now().Add(time.Second)); e != nil || len(fake.events) != 4 {
		t.Fatal(e, len(fake.events))
	}
	for _, event := range fake.events {
		if event.Kind == "sync_summary" {
			v := event.variables()
			if v["failure_count"] != "0" || strings.Contains(v["summary"], "失败目标") || v["title"] != "规则同步成功" {
				t.Fatal("取消订阅后未重建合并消息", v)
			}
		}
	}
}

func TestNotificationSuccessRequiresActualChangesAndExcludesRevoke(t *testing.T) {
	a, _ := testApp(t)
	c := notificationFixture(t, a, "sync_succeeded", "sync_failed")
	target := sourceTarget(t, a)
	result, e := a.syncTarget(context.Background(), target, "8.8.8.8", "manual", false)
	if e != nil || notificationJobCount(t, a) != 1 {
		t.Fatal("实际新增未通知", e)
	}
	event := queuedNotification(t, a, c.Channel.ID)
	if event.Kind != "sync_succeeded" || len(event.Changes) != 2 {
		t.Fatal("成功通知缺少变更", event)
	}
	if _, e := a.syncTarget(context.Background(), result, "8.8.8.8", "manual", false); e != nil || notificationJobCount(t, a) != 1 {
		t.Fatal("规则无需修改却产生成功通知", e)
	}
	result.Note = "更新备注"
	result, e = a.syncTarget(context.Background(), result, "8.8.8.8", "manual", false)
	if e != nil || notificationJobCount(t, a) != 2 {
		t.Fatal("修改备注未产生成功通知", e)
	}
	if _, e := a.syncTarget(context.Background(), result, "", "manual", true); e != nil || notificationJobCount(t, a) != 2 {
		t.Fatal("撤销产生额外同步通知", e)
	}
}

func TestNotificationManualAndScheduledBatches(t *testing.T) {
	for _, trigger := range []string{"manual", "auto"} {
		t.Run(trigger, func(t *testing.T) {
			a, cloud := testApp(t)
			setQuerySettings(t, a, false, false)
			setCachedIP(t, a)
			for i := 0; i < 2; i++ {
				target := sourceTarget(t, a)
				target.AutoSync = true
				if trigger == "auto" {
					if _, e := a.syncTarget(context.Background(), target, "1.1.1.1", "manual", false); e != nil {
						t.Fatal(e)
					}
				} else if e := a.store.saveTarget(target); e != nil {
					t.Fatal(e)
				}
			}
			c := notificationFixture(t, a, "sync_succeeded", "sync_failed")
			cookie := loginCookie(t, a)
			run := func() {
				if trigger == "auto" {
					a.syncScheduled(context.Background())
				} else if w := request(t, a, "POST", "/api/sync", nil, cookie); w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			run()
			if notificationJobCount(t, a) != 1 || queuedNotification(t, a, c.Channel.ID).variables()["success_count"] != "2" {
				t.Fatal("批量同步未合并两个目标")
			}
			cloud.calls = nil
			run()
			if notificationJobCount(t, a) != 1 || trigger == "auto" && len(cloud.calls) != 0 {
				t.Fatal("未变化时重复通知或定时查询云端")
			}
		})
	}
}

func TestNotificationSummaryTemplatesAndBoundedDetails(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	for kind := range notificationTemplateNames {
		w := request(t, a, "POST", "/api/notification-channels/preview", map[string]any{"event": kind}, cookie)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "[测试]") {
			t.Fatal("模板预览失败", kind, w.Code, w.Body.String())
		}
	}
	input := notificationInput{Name: "全部事件", Type: "feishu", Enabled: true, Webhook: testFeishuWebhook, Events: []string{"ip_changed", "sync_succeeded", "sync_failed"}, Templates: defaultNotificationTemplates}
	if w := request(t, a, "POST", "/api/notification-channels", input, cookie); w.Code != 200 {
		t.Fatal("三种事件或四份模板无法保存", w.Code, w.Body.String())
	}
	event := sampleNotification("sync_summary")
	for i := 0; i < 100; i++ {
		item := sampleNotification("sync_failed")
		item.TargetName, item.Error = "<at id=all></at>", strings.Repeat("&\"<>", 200)
		event.Items = append(event.Items, item)
	}
	card, e := renderNotificationCard("", event)
	if e != nil {
		t.Fatal("大批量摘要未限制长度", e)
	}
	content := notificationCardText(card)
	if strings.Contains(content, "<at id=all>") || !strings.Contains(content, "更多详情见操作日志") || event.variables()["failure_count"] != "101" {
		t.Fatal("摘要未转义、截断或计数不完整", content)
	}
}
