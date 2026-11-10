package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type notificationRoundKey struct{}
type notificationRound struct{ items []notificationEvent }

// 一轮同步先收集事件，完成后按渠道订阅合并；嵌套同步共用同一轮。
func (a *App) beginNotificationRound(ctx context.Context) (context.Context, func()) {
	if ctx.Value(notificationRoundKey{}) != nil {
		return ctx, func() {}
	}
	round := &notificationRound{}
	return context.WithValue(ctx, notificationRoundKey{}, round), func() {
		if len(round.items) > 0 {
			a.enqueueNotification(notificationEvent{Kind: "sync_summary", Items: round.items, Trigger: round.items[0].Trigger, Time: now()})
		}
	}
}

func (a *App) notify(ctx context.Context, event notificationEvent) {
	event.Error = notificationText(event.Error, 1500)
	if round, ok := ctx.Value(notificationRoundKey{}).(*notificationRound); ok {
		round.items = append(round.items, event)
		return
	}
	a.enqueueNotification(event)
}

func notificationTargetName(t Target) string {
	if t.Name != "" {
		return t.Name
	}
	if t.GroupName != "" {
		return t.GroupName
	}
	return t.GroupID
}

func notificationText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…（更多详情见操作日志）"
	}
	return text
}

// 入队和实际发送前均筛选，取消订阅后不会在合并卡片中泄露已取消的事件。
func subscribedNotification(event notificationEvent, subscriptions []string) (notificationEvent, bool) {
	if len(event.Items) == 0 {
		return event, slices.Contains(subscriptions, event.Kind)
	}
	items := []notificationEvent{}
	for _, item := range event.Items {
		if slices.Contains(subscriptions, item.Kind) {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return event, false
	}
	if len(items) == 1 {
		item := items[0]
		item.ID, item.Time, item.Timezone, item.SiteURL = event.ID, event.Time, event.Timezone, event.SiteURL
		item.Items = items
		return item, true
	}
	// 从原始子事件重建，避免发送前取消订阅后残留旧的 IP、计数或摘要。
	merged := notificationEvent{ID: event.ID, Kind: "sync_summary", Time: event.Time, Timezone: event.Timezone, Trigger: event.Trigger, SiteURL: event.SiteURL, Items: items}
	for _, item := range items {
		if item.Kind == "ip_changed" {
			merged.OldIP, merged.NewIP = item.OldIP, item.NewIP
		}
	}
	return merged, true
}

func (e notificationEvent) summaryBlocks(markdown bool) (success, failed int, blocks []string) {
	items := e.Items
	if len(items) == 0 {
		items = []notificationEvent{e}
	}
	type line struct {
		text, color   string
		bold, deleted bool
	}
	remaining, omitted := 2000, false
	for _, item := range items {
		if item.Kind == "ip_changed" {
			continue
		}
		lines := []line{}
		if item.Kind == "sync_failed" {
			failed++
			lines = append(lines, line{text: "失败 · " + item.TargetName, color: "red", bold: true})
		} else if item.Kind == "sync_succeeded" {
			success++
			lines = append(lines, line{text: fmt.Sprintf("成功 · %s（%d 项变更）", item.TargetName, len(item.Changes)), color: "green", bold: true})
		} else {
			continue
		}
		if remaining <= 0 || len(blocks) >= 10 {
			omitted = true
			continue
		}
		lines = append(lines, line{text: item.ResourceID + " / " + item.Region, color: "grey"})
		if item.Kind == "sync_failed" {
			reason := strings.Join(strings.Fields(item.Error), " ")
			for _, suffix := range []string{"；接口", "; 接口", "；RequestId", "; RequestId"} {
				if index := strings.Index(reason, suffix); index >= 0 {
					reason = reason[:index]
				}
			}
			lines = append(lines, line{text: "原因：" + notificationText(reason, 100), color: "red"})
		} else {
			for i, change := range item.Changes {
				if i == 3 {
					lines = append(lines, line{text: "其余变更见操作日志", color: "grey"})
					break
				}
				deleted := strings.HasPrefix(change, "- 删除 ")
				color := "green"
				if deleted {
					color = "red"
				}
				// 去掉列表前缀，避免飞书将后续目标和完成信息吞进同一个列表。
				text := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(change, "+ "), "- "), "~ ")
				lines = append(lines, line{text: text, color: color, deleted: deleted})
			}
		}
		parts := []string{}
		for _, line := range lines {
			if remaining <= 0 {
				omitted = true
				break
			}
			text := notificationText(line.text, remaining)
			remaining -= len([]rune(line.text)) + 1
			if markdown {
				// 先转义事件数据，再添加固定样式，目标和错误内容不能注入卡片标记。
				text = notificationMarkdownText(text)
				if line.bold {
					text = "**" + text + "**"
				}
				if line.deleted {
					text = "~~" + text + "~~"
				}
				text = "<font color='" + line.color + "'>" + text + "</font>"
			}
			parts = append(parts, text)
		}
		blocks = append(blocks, strings.Join(parts, "\n\n"))
	}
	if omitted {
		blocks = append(blocks, "更多详情见操作日志")
	}
	return success, failed, blocks
}

func (e notificationEvent) syncSummary() (success, failed int, summary string) {
	success, failed, blocks := e.summaryBlocks(false)
	return success, failed, strings.Join(blocks, "\n\n")
}
