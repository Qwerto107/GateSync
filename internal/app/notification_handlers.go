package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

type notificationInput struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Enabled         bool              `json:"enabled"`
	Events          []string          `json:"events"`
	Templates       map[string]string `json:"templates"`
	Webhook         string            `json:"webhook"`
	SignSecret      string            `json:"signSecret"`
	ClearSignSecret bool              `json:"clearSignSecret"`
	Event           string            `json:"event"`
}

func (a *App) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	configs, e := a.store.notificationChannels()
	if e != nil {
		fail(w, 500, "读取通知渠道失败")
		return
	}
	items := []notificationChannel{}
	for _, c := range configs {
		items = append(items, c.Channel)
	}
	jsonResponse(w, 200, map[string]any{"items": items, "events": notificationEventNames, "templateNames": notificationTemplateNames, "defaultTemplates": defaultNotificationTemplates, "variables": notificationVariables})
}

func (a *App) notificationConfig(input notificationInput) (notificationChannelConfig, error) {
	c := notificationChannelConfig{}
	if input.ID != "" {
		var e error
		c, e = a.store.notificationChannel(input.ID)
		if e != nil {
			return c, e
		}
	} else {
		c.Channel.ID = randomID()
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 60 {
		return c, fmt.Errorf("渠道名称须为 1–60 个字符")
	}
	if input.Type != "feishu" {
		return c, fmt.Errorf("当前仅支持飞书通知渠道")
	}
	if len(input.Events) > len(notificationEventNames) || input.Enabled && len(input.Events) == 0 {
		return c, fmt.Errorf("请选择需要通知的事件")
	}
	events := []string{}
	for _, event := range input.Events {
		if notificationEventNames[event] == "" {
			return c, fmt.Errorf("通知事件无效")
		}
		if !slices.Contains(events, event) {
			events = append(events, event)
		}
	}
	if len(input.Templates) > len(notificationTemplateNames) {
		return c, fmt.Errorf("通知模板类型无效")
	}
	templates := map[string]string{}
	for kind, raw := range input.Templates {
		if notificationTemplateNames[kind] == "" {
			return c, fmt.Errorf("通知模板类型无效")
		}
		if _, e := renderNotificationCard(raw, sampleNotification(kind)); e != nil {
			return c, e
		}
		if strings.TrimSpace(raw) != "" {
			templates[kind] = raw
		}
	}
	if len(input.Webhook) > 1024 || len(input.SignSecret) > 512 {
		return c, fmt.Errorf("通知渠道凭证内容过长")
	}
	if input.ClearSignSecret && input.SignSecret != "" {
		return c, fmt.Errorf("不能同时设置和清除签名密钥")
	}
	if input.Webhook != "" {
		c.Webhook = strings.TrimSpace(input.Webhook)
	}
	if e := validateFeishuWebhook(c.Webhook); e != nil {
		return c, e
	}
	if input.ClearSignSecret {
		c.SignSecret = ""
	} else if input.SignSecret != "" {
		c.SignSecret = strings.TrimSpace(input.SignSecret)
	}
	c.Channel = notificationChannel{ID: c.Channel.ID, Name: name, Type: input.Type, Enabled: input.Enabled, Events: events, Templates: templates, HasWebhook: true, HasSignSecret: c.SignSecret != ""}
	return c, nil
}

func notificationConfigError(w http.ResponseWriter, e error) {
	if errors.Is(e, sql.ErrNoRows) {
		fail(w, 404, "通知渠道不存在")
		return
	}
	// 校验错误只由本地固定文案构成，凭证和服务端原始响应不会进入错误文本。
	fail(w, 400, e.Error())
}

func (a *App) saveNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var input notificationInput
	if !decode(w, r, &input) {
		return
	}
	if input.ID != "" && input.ID != r.PathValue("id") {
		fail(w, 400, "渠道 ID 与请求路径不一致")
		return
	}
	input.ID = r.PathValue("id")
	a.mutate.Lock()
	defer a.mutate.Unlock()
	if input.ID == "" {
		var count int
		if e := a.store.db.QueryRow("SELECT COUNT(*) FROM notification_channels").Scan(&count); e != nil {
			fail(w, 500, "读取通知渠道失败")
			return
		}
		if count >= 20 {
			fail(w, 400, "最多添加 20 个通知渠道")
			return
		}
	}
	c, e := a.notificationConfig(input)
	if e != nil {
		notificationConfigError(w, e)
		return
	}
	if e := a.store.saveNotificationChannel(c); e != nil {
		fail(w, 500, "保存通知渠道失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "settings", Trigger: "manual", Object: c.Channel.Name, Detail: "保存飞书通知渠道", Success: true})
	jsonResponse(w, 200, c.Channel)
}

func (a *App) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	c, e := a.store.notificationChannel(r.PathValue("id"))
	if e != nil {
		notificationConfigError(w, e)
		return
	}
	if _, e := a.store.db.Exec("DELETE FROM notification_channels WHERE id=?", c.Channel.ID); e != nil {
		fail(w, 500, "删除通知渠道失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "settings", Trigger: "manual", Object: c.Channel.Name, Detail: "删除通知渠道及其待发送任务", Success: true})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}

func (a *App) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var input notificationInput
	if !decode(w, r, &input) {
		return
	}
	if notificationTemplateNames[input.Event] == "" {
		fail(w, 400, "请选择测试事件")
		return
	}
	c, e := a.notificationConfig(input)
	if e != nil {
		notificationConfigError(w, e)
		return
	}
	event := a.prepareNotification(sampleNotification(input.Event))
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	e = a.notificationSenders[c.Channel.Type].Send(ctx, c, event)
	detail := "飞书测试卡片已发送"
	if e != nil {
		detail = e.Error()
	}
	_ = a.store.addLog(LogEntry{Kind: "notification", Trigger: "manual", Object: c.Channel.Name, Detail: detail, Success: e == nil})
	if e != nil {
		fail(w, 502, detail)
		return
	}
	jsonResponse(w, 200, map[string]string{"message": detail})
}

func (a *App) previewNotification(w http.ResponseWriter, r *http.Request) {
	var input notificationInput
	if !decode(w, r, &input) {
		return
	}
	if notificationTemplateNames[input.Event] == "" {
		fail(w, 400, "请选择预览事件")
		return
	}
	card, e := renderNotificationCard(input.Templates[input.Event], a.prepareNotification(sampleNotification(input.Event)))
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	jsonResponse(w, 200, map[string]any{"card": card})
}
