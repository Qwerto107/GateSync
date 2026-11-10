package app

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var notificationEventNames = map[string]string{"ip_changed": "公网 IP 变更", "sync_succeeded": "同步成功（实际变更）", "sync_failed": "同步失败"}
var notificationTemplateNames = map[string]string{"ip_changed": "公网 IP 变更", "sync_succeeded": "同步成功", "sync_failed": "同步失败", "sync_summary": "合并同步结果"}
var notificationVariables = []string{"title", "old_ip", "new_ip", "time", "timezone", "target_name", "resource_id", "region", "error", "trigger", "site_url", "success_count", "failure_count", "summary", "ip_summary"}
var notificationPlaceholder = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

var defaultNotificationTemplates = map[string]string{
	"sync_succeeded": `{
  "schema": "2.0",
  "header": {"title": {"tag": "plain_text", "content": "{{title}}"}, "template": "green"},
  "body": {"elements": [
    {"tag": "markdown", "content": "{{ip_summary}}"},
    {"tag": "markdown", "content": "{{summary}}"},
    {"tag": "markdown", "content": "<font color='grey'>完成时间：{{time}}（{{timezone}}） · {{trigger}}</font>"}
  ]}
}`,
	"sync_summary": `{
  "schema": "2.0",
  "header": {"title": {"tag": "plain_text", "content": "{{title}}"}, "template": "blue"},
  "body": {"elements": [
    {"tag": "markdown", "content": "{{ip_summary}}"},
    {"tag": "markdown", "content": "<font color='green'>成功变更 {{success_count}} 个目标</font>　·　<font color='red'>失败 {{failure_count}} 个目标</font>"},
    {"tag": "markdown", "content": "{{summary}}"},
    {"tag": "markdown", "content": "<font color='grey'>完成时间：{{time}}（{{timezone}}） · {{trigger}}</font>"}
  ]}
}`,
	"ip_changed": `{
  "schema": "2.0",
  "header": {"title": {"tag": "plain_text", "content": "{{title}}"}, "template": "blue"},
  "body": {"elements": [
    {"tag": "markdown", "content": "旧 IP：<font color='red'>~~{{old_ip}}~~</font>　→　新 IP：<font color='green'>{{new_ip}}</font>"},
    {"tag": "markdown", "content": "<font color='grey'>更新时间：{{time}}（{{timezone}}）</font>"}
  ]}
}`,
	"sync_failed": `{
  "schema": "2.0",
  "header": {"title": {"tag": "plain_text", "content": "{{title}}"}, "template": "red"},
  "body": {"elements": [
    {"tag": "markdown", "content": "{{summary}}"},
    {"tag": "markdown", "content": "<font color='grey'>发生时间：{{time}}（{{timezone}}） · {{trigger}}</font>"}
  ]}
}`,
}

type notificationEvent struct {
	ID         string              `json:"id"`
	Kind       string              `json:"kind"`
	OldIP      string              `json:"oldIp"`
	NewIP      string              `json:"newIp"`
	Time       string              `json:"time"`
	Timezone   string              `json:"timezone"`
	TargetName string              `json:"targetName"`
	ResourceID string              `json:"resourceId"`
	Region     string              `json:"region"`
	Error      string              `json:"error"`
	Trigger    string              `json:"trigger"`
	SiteURL    string              `json:"siteUrl"`
	Test       bool                `json:"test"`
	Changes    []string            `json:"changes,omitempty"`
	Items      []notificationEvent `json:"items,omitempty"`
}

func (e notificationEvent) variables() map[string]string {
	zone, err := time.LoadLocation(e.Timezone)
	if err != nil {
		zone = time.UTC
	}
	at, _ := time.Parse(time.RFC3339, e.Time)
	title := "公网 IP 更新"
	success, failed, summary := e.syncSummary()
	if e.Kind == "sync_succeeded" {
		title = "规则同步成功"
	}
	if e.Kind == "sync_failed" {
		title = "规则同步失败"
	}
	if e.Kind == "sync_summary" {
		title = "规则同步成功"
		if failed > 0 {
			title = "规则同步失败"
			if success > 0 {
				title = "规则同步部分失败"
			}
		}
	}
	if e.Test {
		title = "[测试] " + title
	}
	trigger := "手动"
	if e.Trigger == "auto" {
		trigger = "自动"
	}
	if e.Test {
		trigger = "测试"
	}
	ipSummary := ""
	if e.OldIP != "" && e.NewIP != "" && e.OldIP != e.NewIP {
		ipSummary = "公网 IP：" + e.OldIP + " → " + e.NewIP
	}
	return map[string]string{"title": title, "old_ip": e.OldIP, "new_ip": e.NewIP, "time": at.In(zone).Format("2006-01-02 15:04:05"), "timezone": e.Timezone, "target_name": e.TargetName, "resource_id": e.ResourceID, "region": e.Region, "error": e.Error, "trigger": trigger, "site_url": e.SiteURL, "success_count": strconv.Itoa(success), "failure_count": strconv.Itoa(failed), "summary": summary, "ip_summary": ipSummary}
}

func sampleNotification(kind string) notificationEvent {
	event := notificationEvent{Kind: kind, OldIP: "8.8.8.8", NewIP: "9.9.9.9", Time: now(), Timezone: "Asia/Shanghai", TargetName: "办公室 SSH", ResourceID: "sg-example", Region: "ap-guangzhou", Error: "示例：云端权限不足，请检查对应 CAM 授权", Trigger: "manual", SiteURL: "http://localhost:13850", Test: true, Changes: []string{"+ 新增 TCP 22 9.9.9.9/32", "- 删除 TCP 22 8.8.8.8/32"}}
	if kind == "sync_summary" {
		succeeded, failed := event, event
		succeeded.Kind = "sync_succeeded"
		failed.Kind, failed.TargetName, failed.ResourceID = "sync_failed", "轻量服务器 Web", "lhins-example"
		event.Items = []notificationEvent{succeeded, failed}
	}
	return event
}

func notificationMarkdownText(text string) string {
	text = html.EscapeString(text)
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "~", "\\~", "`", "\\`", "[", "\\[", "]", "\\]").Replace(text)
}

func renderNotificationCard(raw string, event notificationEvent) (map[string]any, error) {
	if raw == "" {
		raw = defaultNotificationTemplates[event.Kind]
	}
	if len(raw) > 16*1024 {
		return nil, fmt.Errorf("卡片模板不能超过 16 KiB")
	}
	var card map[string]any
	if json.Unmarshal([]byte(raw), &card) != nil || card == nil || card["schema"] != "2.0" {
		return nil, fmt.Errorf("请输入飞书卡片 JSON 2.0 对象，不包含 msg_type 外层")
	}
	body, ok := card["body"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("卡片模板缺少 body")
	}
	elements, ok := body["elements"].([]any)
	if !ok || len(elements) == 0 {
		return nil, fmt.Errorf("卡片模板需要非空的 body.elements")
	}
	values := event.variables()
	_, _, summaryBlocks := event.summaryBlocks(true)
	// 完整的摘要占位元素展开为各目标独立的 Markdown 组件，保留飞书组件间距。
	summaryElements := map[int]bool{}
	for i, value := range elements {
		if element, ok := value.(map[string]any); ok && element["tag"] == "markdown" && element["content"] == "{{summary}}" {
			summaryElements[i] = true
		}
	}
	var walk func(any, bool) (any, error)
	walk = func(value any, markdown bool) (any, error) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if strings.Contains(key, "{{") {
					return nil, fmt.Errorf("模板变量只能用于 JSON 字符串值")
				}
				rendered, err := walk(child, key == "content" && (v["tag"] == "markdown" || v["tag"] == "lark_md"))
				if err != nil {
					return nil, err
				}
				v[key] = rendered
			}
		case []any:
			for i, child := range v {
				rendered, err := walk(child, false)
				if err != nil {
					return nil, err
				}
				v[i] = rendered
			}
		case string:
			var invalid bool
			// 先解析 JSON 再替换字符串；事件数据不会被当作模板或 JSON 代码执行。
			result := notificationPlaceholder.ReplaceAllStringFunc(v, func(token string) string {
				name := notificationPlaceholder.FindStringSubmatch(token)[1]
				s, ok := values[name]
				if !ok {
					invalid = true
					return ""
				}
				if markdown {
					s = notificationMarkdownText(s)
					if name == "summary" {
						s = strings.Join(summaryBlocks, "\n\n")
					} else if name == "ip_summary" && s != "" {
						s = "公网 IP：<font color='red'>~~" + notificationMarkdownText(event.OldIP) + "~~</font> → <font color='green'>" + notificationMarkdownText(event.NewIP) + "</font>"
					}
				}
				return s
			})
			// 校验原模板，不检查事件中恰好含有花括号的普通文本。
			remaining := notificationPlaceholder.ReplaceAllString(v, "")
			if invalid || strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
				return nil, fmt.Errorf("模板含未知或无效变量，请参考变量说明")
			}
			return result, nil
		}
		return value, nil
	}
	if _, err := walk(card, false); err != nil {
		return nil, err
	}
	expanded := []any{}
	for i, value := range elements {
		if summaryElements[i] {
			original := value.(map[string]any)
			for _, block := range summaryBlocks {
				element := map[string]any{}
				for key, child := range original {
					element[key] = child
				}
				element["content"] = block
				expanded = append(expanded, element)
			}
		} else {
			if element, ok := value.(map[string]any); ok && element["tag"] == "markdown" && element["content"] == "" {
				continue
			}
			expanded = append(expanded, value)
		}
	}
	body["elements"] = expanded
	encoded, err := json.Marshal(card)
	if err != nil || len(encoded) > 19*1024 {
		return nil, fmt.Errorf("渲染后的卡片内容过长")
	}
	return card, nil
}
