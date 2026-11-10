package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLogTimeRangeFiltersGroupsDetailsAndCSV(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	from := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	to := from.Add(30 * time.Minute)
	for _, at := range []time.Time{from.Add(-time.Second), from, to.Add(-time.Second), to} {
		if e := a.store.addLog(LogEntry{Time: at.Format(time.RFC3339), Kind: "ip", Trigger: "auto", Object: "公网 IP", Detail: "公网 IP 无变化：8.8.8.8", Success: true}); e != nil {
			t.Fatal(e)
		}
	}
	// 同一范围使用不同偏移，验证按绝对时间比较且不叠加旧 days 默认值。
	query := url.Values{"from": {from.In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339)}, "to": {to.In(time.FixedZone("UTC-5", -5*3600)).Format(time.RFC3339)}}
	for _, grouped := range []bool{false, true} {
		query.Set("group", map[bool]string{true: "true", false: "false"}[grouped])
		w := request(t, a, "GET", "/api/logs?"+query.Encode(), nil, cookie)
		var out struct {
			Items    []logGroup `json:"items"`
			Total    int        `json:"total"`
			RawTotal int        `json:"rawTotal"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || w.Code != 200 {
			t.Fatal(w.Code, w.Body.String(), e)
		}
		if grouped {
			if out.Total != 1 || out.RawTotal != 2 || out.Items[0].Count != 2 || out.Items[0].FirstTime != from.Format(time.RFC3339) {
				t.Fatal("聚合未截取所选时间范围", out)
			}
			query.Set("firstId", "1")
			query.Set("lastId", "99999")
		} else if out.Total != 2 || len(out.Items) != 2 {
			t.Fatal("起止边界或偏移处理错误", out)
		}
	}
	query.Set("group", "false")
	w := request(t, a, "GET", "/api/logs?"+query.Encode(), nil, cookie)
	if !strings.Contains(w.Body.String(), `"total":2`) {
		t.Fatal("明细未沿用相同时间范围", w.Body.String())
	}
	w = request(t, a, "GET", "/api/logs/export?"+query.Encode(), nil, cookie)
	rows, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	if e != nil || w.Code != 200 || len(rows) != 3 {
		t.Fatal("导出时间范围不一致", e, w.Body.String())
	}
	query.Set("from", from.Add(500*time.Millisecond).Format(time.RFC3339Nano))
	w = request(t, a, "GET", "/api/logs?"+query.Encode(), nil, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatal("毫秒边界错误", w.Body.String())
	}
	for _, values := range []url.Values{
		{"from": {from.Format(time.RFC3339)}},
		{"from": {"invalid"}, "to": {to.Format(time.RFC3339)}},
		{"from": {to.Format(time.RFC3339)}, "to": {from.Format(time.RFC3339)}},
		{"from": {from.Format(time.RFC3339)}, "to": {from.Format(time.RFC3339)}},
	} {
		for _, path := range []string{"/api/logs?", "/api/logs/export?"} {
			if w := request(t, a, "GET", path+values.Encode(), nil, cookie); w.Code != 400 {
				t.Fatal("接受无效时间范围", values, path, w.Code)
			}
		}
	}
}

func TestLogRetentionDefaultsMigrationAndSettings(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	w := request(t, a, "GET", "/api/log-settings", nil, cookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"retentionDays":180`) {
		t.Fatal("新安装未默认 180 天", w.Body.String())
	}
	for _, body := range []any{map[string]any{}, map[string]any{"retentionDays": -1}, map[string]any{"retentionDays": 36501}, map[string]any{"retentionDays": 1.5}} {
		if w := request(t, a, "PUT", "/api/log-settings", body, cookie); w.Code != 400 {
			t.Fatal("接受无效保留天数", body, w.Code)
		}
	}
	w = request(t, a, "PUT", "/api/log-settings", map[string]any{"retentionDays": 0}, cookie)
	if w.Code != 200 || request(t, a, "GET", "/api/state", nil, cookie).Code != 200 {
		t.Fatal("独立保存失败或撤销了会话", w.Body.String())
	}
	if w := request(t, a, "PUT", "/api/log-settings", map[string]any{"retentionDays": 1}, nil); w.Code != 401 {
		t.Fatal("保留设置绕过登录", w.Code)
	}
	if _, e := a.store.db.Exec("DELETE FROM meta WHERE key='log-settings'"); e != nil {
		t.Fatal(e)
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
	var settings logSettings
	if e := restarted.store.getMeta("log-settings", &settings); e != nil || settings.RetentionDays != 0 {
		t.Fatal("已有安装未迁移为永久保留", settings, e)
	}
}

func TestDailyLogCleanupBoundariesAndRestart(t *testing.T) {
	a, _ := testApp(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	cutoff := at.AddDate(0, 0, -10)
	if e := a.store.setMeta("log-settings", logSettings{RetentionDays: 10}); e != nil {
		t.Fatal(e)
	}
	add := func(at time.Time) {
		t.Helper()
		if e := a.store.addLog(LogEntry{Time: at.Format(time.RFC3339), Kind: "ip", Detail: "检测失败", Success: false}); e != nil {
			t.Fatal(e)
		}
	}
	count := func() int {
		t.Helper()
		var n int
		if e := a.store.db.QueryRow("SELECT count(*) FROM logs").Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	add(cutoff.Add(-time.Second))
	add(cutoff)
	add(at)
	if next, e := a.cleanupLogs(ctx, at); e != nil || !next.Equal(at.Add(24*time.Hour)) || count() != 2 {
		t.Fatal("清理边界错误", next, e, count())
	}
	add(cutoff.Add(-time.Second))
	if _, e := a.cleanupLogs(ctx, at.Add(time.Hour)); e != nil || count() != 3 {
		t.Fatal("每日清理被重复执行", e, count())
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
	if next, e := restarted.cleanupLogs(ctx, at.Add(2*time.Hour)); e != nil || !next.Equal(at.Add(24*time.Hour)) {
		t.Fatal("重启未保留清理计划", next, e)
	}
	if _, e := restarted.cleanupLogs(ctx, at.Add(48*time.Hour)); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := restarted.store.db.QueryRow("SELECT count(*) FROM logs").Scan(&n); e != nil || n != 1 {
		t.Fatal("未补做过期清理", n, e)
	}
	if e := restarted.store.setMeta("log-settings", logSettings{RetentionDays: 0}); e != nil {
		t.Fatal(e)
	}
	if _, e := restarted.cleanupLogs(ctx, at.AddDate(1, 0, 0)); e != nil {
		t.Fatal(e)
	}
	if e := restarted.store.db.QueryRow("SELECT count(*) FROM logs").Scan(&n); e != nil || n != 1 {
		t.Fatal("永久保留仍删除了日志", n, e)
	}
}
