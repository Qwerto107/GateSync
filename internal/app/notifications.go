package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"
)

func (a *App) prepareNotification(event notificationEvent) notificationEvent {
	if event.ID == "" {
		event.ID = randomID()
	}
	if event.Time == "" {
		event.Time = now()
	}
	settings, e := a.settings()
	if e == nil {
		event.Timezone = settings.Timezone
	}
	if event.Timezone == "" {
		event.Timezone = "Asia/Shanghai"
	}
	event.SiteURL = a.systemSettings().PublicURL
	if len([]rune(event.Error)) > 1500 {
		event.Error = string([]rune(event.Error)[:1500]) + "…"
	}
	return event
}

func (a *App) enqueueNotification(event notificationEvent) {
	event = a.prepareNotification(event)
	channels, e := a.store.notificationChannels()
	if e != nil {
		log.Printf("读取通知渠道失败，本次通知未入队")
		return
	}
	for _, c := range channels {
		filtered, subscribed := subscribedNotification(event, c.Channel.Events)
		if !c.Channel.Enabled || !subscribed {
			continue
		}
		raw, e := json.Marshal(filtered)
		if e != nil {
			continue
		}
		// 一轮同步为每个渠道只创建一个任务，事件按该渠道的订阅合并。
		if _, e := a.store.db.Exec("INSERT INTO notification_jobs(id,channel_id,event,next_attempt) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING", event.ID+":"+c.Channel.ID, c.Channel.ID, string(raw), time.Now().Unix()); e != nil {
			_ = a.store.addLog(LogEntry{Kind: "notification", Trigger: event.Trigger, Object: c.Channel.Name, Detail: "通知入队失败", Success: false})
		}
	}
}

func (a *App) runNotifications(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if e := a.deliverNotifications(ctx, time.Now()); e != nil && ctx.Err() == nil {
				log.Printf("处理通知队列失败，将在后续轮次重试")
			}
		}
	}
}

func (a *App) deliverNotifications(ctx context.Context, at time.Time) error {
	type job struct {
		id, channelID, raw string
		attempts           int
	}
	rows, e := a.store.db.QueryContext(ctx, "SELECT id,channel_id,event,attempts FROM notification_jobs WHERE next_attempt<=? ORDER BY next_attempt,id LIMIT 10", at.Unix())
	if e != nil {
		return e
	}
	jobs := []job{}
	for rows.Next() {
		var j job
		if e = rows.Scan(&j.id, &j.channelID, &j.raw, &j.attempts); e != nil {
			break
		}
		jobs = append(jobs, j)
	}
	readErr := rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if readErr != nil {
		return readErr
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c, e := a.store.notificationChannel(j.channelID)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		var event notificationEvent
		if e = json.Unmarshal([]byte(j.raw), &event); e != nil {
			return e
		}
		event, subscribed := subscribedNotification(event, c.Channel.Events)
		if !c.Channel.Enabled || !subscribed {
			if _, e := a.store.db.ExecContext(ctx, "DELETE FROM notification_jobs WHERE id=?", j.id); e != nil {
				return e
			}
			continue
		}
		sender := a.notificationSenders[c.Channel.Type]
		if sender == nil {
			return errors.New("通知渠道类型不支持")
		}
		// 发送在独立任务中执行，不占用检测与规则同步的互斥锁。
		sendCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		sendErr := sender.Send(sendCtx, c, event)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		attempt := j.attempts + 1
		detail := notificationTemplateNames[event.Kind] + "通知已发送"
		if sendErr == nil || attempt >= 3 {
			if _, e = a.store.db.ExecContext(ctx, "DELETE FROM notification_jobs WHERE id=?", j.id); e != nil {
				return e
			}
			if sendErr != nil {
				detail = sendErr.Error() + "；已尝试 3 次，停止自动重试"
			}
		} else {
			delay := 30 * time.Second
			if attempt == 2 {
				delay = 2 * time.Minute
			}
			_, e = a.store.db.ExecContext(ctx, "UPDATE notification_jobs SET attempts=?,next_attempt=? WHERE id=?", attempt, at.Add(delay).Unix(), j.id)
			if e != nil {
				return e
			}
			detail = sendErr.Error() + "；稍后自动重试"
		}
		_ = a.store.addLog(LogEntry{Kind: "notification", Trigger: event.Trigger, Object: c.Channel.Name, Detail: detail, Success: sendErr == nil})
	}
	return nil
}
