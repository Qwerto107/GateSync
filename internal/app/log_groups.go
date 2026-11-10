package app

import (
	"encoding/json"
	"net/http"
)

type logGroup struct {
	LogEntry
	Count     int    `json:"count"`
	FirstID   int64  `json:"firstId"`
	FirstTime string `json:"firstTime"`
}

func (a *App) groupedLogs(w http.ResponseWriter, r *http.Request, f logFilter, page int) {
	// 先在完整日志顺序上划分连续段，再筛选和分页，避免跨过失败、变更或其他操作合并。
	query := `WITH numbered AS (
		SELECT *, ROW_NUMBER() OVER (ORDER BY id) -
		ROW_NUMBER() OVER (PARTITION BY kind,trigger,object,detail,success,changes ORDER BY id) AS run
		FROM logs
	), filtered AS (
		SELECT *, CASE WHEN kind='ip' AND success=1 AND changes='[]'
		AND detail LIKE '公网 IP 无变化：%' THEN 1 ELSE 0 END AS collapsible
		FROM numbered` + f.where + `
	), summaries AS (
		SELECT MAX(id) AS id, MIN(id) AS first_id, COUNT(*) AS count, MIN(time) AS first_time
		FROM filtered
		GROUP BY collapsible, CASE WHEN collapsible=1 THEN run ELSE id END,
		kind,trigger,object,detail,success,changes
	) `
	tx, e := a.store.db.BeginTx(r.Context(), nil)
	if e != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	defer tx.Rollback()
	var total, rawTotal int
	if e = tx.QueryRowContext(r.Context(), query+"SELECT COUNT(*), COALESCE(SUM(count),0) FROM summaries", f.args...).Scan(&total, &rawTotal); e != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	args := append(append([]any{}, f.args...), 20, (page-1)*20)
	rows, e := tx.QueryContext(r.Context(), query+`SELECT l.id,l.time,l.kind,l.trigger,l.object,l.detail,l.success,l.changes,
		s.count,s.first_id,s.first_time FROM summaries s JOIN logs l ON l.id=s.id
		ORDER BY l.id DESC LIMIT ? OFFSET ?`, args...)
	if e != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	defer rows.Close()
	items := []logGroup{}
	for rows.Next() {
		var l logGroup
		var raw string
		if e = rows.Scan(&l.ID, &l.Time, &l.Kind, &l.Trigger, &l.Object, &l.Detail, &l.Success, &raw, &l.Count, &l.FirstID, &l.FirstTime); e != nil {
			break
		}
		if e = json.Unmarshal([]byte(raw), &l.Changes); e != nil {
			break
		}
		items = append(items, l)
	}
	if e != nil || rows.Err() != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	jsonResponse(w, 200, map[string]any{"items": items, "total": total, "rawTotal": rawTotal, "page": page})
}
