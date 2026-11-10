package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type logSettings struct {
	RetentionDays int `json:"retentionDays"`
}

func (a *App) initLogSettings(newInstallation bool) error {
	var settings logSettings
	e := a.store.getMeta("log-settings", &settings)
	if errors.Is(e, sql.ErrNoRows) {
		// 升级时保留全部历史记录，只有新安装默认启用 180 天清理。
		if newInstallation {
			settings.RetentionDays = 180
		}
		return a.store.setMeta("log-settings", settings)
	}
	if e == nil && (settings.RetentionDays < 0 || settings.RetentionDays > 36500) {
		return fmt.Errorf("日志保留天数无效")
	}
	return e
}

func (a *App) getLogSettings(w http.ResponseWriter, r *http.Request) {
	var settings logSettings
	if e := a.store.getMeta("log-settings", &settings); e != nil {
		fail(w, 500, "读取日志保留设置失败")
		return
	}
	jsonResponse(w, 200, settings)
}

func (a *App) updateLogSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RetentionDays *int `json:"retentionDays"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.RetentionDays == nil || *input.RetentionDays < 0 || *input.RetentionDays > 36500 {
		fail(w, 400, "日志保留天数须为 0–36500 的整数，0 表示永久保留")
		return
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	settings := logSettings{RetentionDays: *input.RetentionDays}
	if e := a.store.setMeta("log-settings", settings); e != nil {
		fail(w, 500, "保存日志保留设置失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "settings", Trigger: "manual", Object: "日志保留", Detail: fmt.Sprintf("日志保留天数设为 %d（0 表示永久）", settings.RetentionDays), Success: true})
	jsonResponse(w, 200, settings)
}

func (a *App) cleanupLogs(ctx context.Context, at time.Time) (time.Time, error) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	var settings logSettings
	if e := a.store.getMeta("log-settings", &settings); e != nil {
		return time.Time{}, e
	}
	next := at.Add(24 * time.Hour)
	if settings.RetentionDays == 0 {
		return next, nil
	}
	var last time.Time
	if e := a.store.getMeta("log-cleanup", &last); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return time.Time{}, e
	}
	if at.Before(last.Add(24 * time.Hour)) {
		return last.Add(24 * time.Hour), nil
	}
	tx, e := a.store.db.BeginTx(ctx, nil)
	if e != nil {
		return time.Time{}, e
	}
	defer tx.Rollback()
	// 删除与完成时间一起提交，重启补做过期任务，失败时仍可重试。
	cutoff := at.UTC().AddDate(0, 0, -settings.RetentionDays).Format(time.RFC3339)
	if _, e = tx.ExecContext(ctx, "DELETE FROM logs WHERE time<?", cutoff); e != nil {
		return time.Time{}, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('log-cleanup',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, `"`+at.UTC().Format(time.RFC3339Nano)+`"`); e != nil {
		return time.Time{}, e
	}
	return next, tx.Commit()
}
