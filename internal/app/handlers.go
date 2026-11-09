package app

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) state(w http.ResponseWriter, r *http.Request) {
	accounts, e := a.store.accounts()
	if e != nil {
		fail(w, 500, "读取云账号失败")
		return
	}
	targets, e := a.store.targets()
	if e != nil {
		fail(w, 500, "读取目标失败")
		return
	}
	settings, e := a.settings()
	if e != nil {
		fail(w, 500, "读取设置失败")
		return
	}
	var ip IPState
	_ = a.store.getMeta("ip", &ip)
	for i := range accounts {
		accounts[i].ResourceType = resourceType(accounts[i].ResourceType)
		for _, t := range targets {
			if t.AccountID == accounts[i].ID {
				accounts[i].TargetCount++
			}
		}
	}
	for i, t := range targets {
		targets[i].ResourceType = resourceType(t.ResourceType)
		targets[i].FixedIPs = append([]FixedIP{}, t.fixedIPs()...)
		actual := t.CurrentCIDRs
		if actual == nil && t.CurrentCIDR != "" {
			actual = []string{t.CurrentCIDR}
		}
		targets[i].CurrentCIDRs = append([]string{}, actual...)
		if t.Status == "synced" && !sameCIDRs(actual, t.desiredCIDRs(ip.IP)) {
			targets[i].Status = "outdated"
		}
	}
	a.nextMu.Lock()
	next := ""
	if !a.next.IsZero() {
		next = a.next.UTC().Format(time.RFC3339)
	}
	a.nextMu.Unlock()
	jsonResponse(w, 200, map[string]any{"accounts": accounts, "targets": targets, "settings": settings, "ip": ip, "nextCheck": next, "identity": r.Context().Value(identityKey{})})
}
func (a *App) detect(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	ip, e := a.detectIP(ctx, "manual")
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	jsonResponse(w, 200, ip)
}
func (a *App) saveSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if !decode(w, r, &s) {
		return
	}
	if _, e := s.schedule(); e != nil {
		fail(w, 400, "请输入有效时区和五段 Cron，或选择检查间隔")
		return
	}
	if len(s.Sources) == 0 || len(s.Sources) > 3 {
		fail(w, 400, "请配置 1 至 3 个 IP 检测源")
		return
	}
	for i, source := range s.Sources {
		s.Sources[i] = strings.TrimSpace(source)
		if e := validateSource(s.Sources[i]); e != nil {
			fail(w, 400, e.Error())
			return
		}
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	if e := a.store.setMeta("settings", s); e != nil {
		fail(w, 500, "保存设置失败")
		return
	}
	select {
	case a.wake <- struct{}{}:
	default:
	}
	_ = a.store.addLog(LogEntry{Kind: "settings", Trigger: "manual", Object: "计划任务", Detail: "更新自动同步设置", Success: true})
	jsonResponse(w, 200, s)
}

type accountInput struct {
	ResourceType string `json:"resourceType"`
	Name         string `json:"name"`
	Region       string `json:"region"`
	SecretID     string `json:"secretId"`
	SecretKey    string `json:"secretKey"`
}

func (a *App) resolveCredentials(id string, input accountInput) (Credentials, error) {
	c := Credentials{strings.TrimSpace(input.SecretID), strings.TrimSpace(input.SecretKey)}
	if id != "" {
		old, e := a.store.credentials(id)
		if e != nil {
			return c, e
		}
		if c.SecretID == "" {
			c.SecretID = old.SecretID
		}
		if c.SecretKey == "" {
			c.SecretKey = old.SecretKey
		}
	}
	if c.SecretID == "" || c.SecretKey == "" || len(c.SecretID) > 256 || len(c.SecretKey) > 256 {
		return c, fmt.Errorf("请填写有效的 SecretId 与 SecretKey")
	}
	return c, nil
}
func masked(s string) string {
	if len(s) < 10 {
		return "••••••••"
	}
	return s[:4] + "••••••••" + s[len(s)-4:]
}
func (a *App) saveAccount(w http.ResponseWriter, r *http.Request) {
	var input accountInput
	if !decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 100 || !regionPattern.MatchString(input.Region) || !validResourceType(input.ResourceType) {
		fail(w, 400, "请填写账号名称与有效地域")
		return
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	id := r.PathValue("id")
	if id != "" {
		if _, e := a.store.account(id); e != nil {
			fail(w, 404, "云账号不存在")
			return
		}
	}
	c, e := a.resolveCredentials(id, input)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	if id == "" {
		id = randomID()
	}
	item := Account{ResourceType: resourceType(input.ResourceType), ID: id, Name: input.Name, Provider: "tencent", Region: input.Region, MaskedKey: masked(c.SecretID), Status: "untested"}
	if e = a.store.saveAccount(item, &c); e != nil {
		fail(w, 500, "保存云账号失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "account", Trigger: "manual", Object: item.Name, Detail: "保存云账号与默认地域", Success: true})
	jsonResponse(w, 200, item)
}
func (a *App) testCredentials(w http.ResponseWriter, r *http.Request) {
	var input struct {
		accountInput
		ID string `json:"id"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !regionPattern.MatchString(input.Region) || !validResourceType(input.ResourceType) {
		fail(w, 400, "地域格式无效")
		return
	}
	c, e := a.resolveCredentials(input.ID, input.accountInput)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	cloud, e := a.cloudFor(c, input.Region, input.ResourceType)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	groups, e := cloud.Groups(r.Context())
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	label := "安全组"
	if resourceType(input.ResourceType) == "lighthouse" {
		label = "Lighthouse 实例"
	}
	jsonResponse(w, 200, map[string]any{"count": len(groups), "message": fmt.Sprintf("连接成功，可见 %d 个%s；写入权限将在实际同步时验证", len(groups), label)})
}
func (a *App) testAccount(w http.ResponseWriter, r *http.Request) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	item, e := a.store.account(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "云账号不存在")
		return
	}
	c, e := a.store.credentials(item.ID)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	cloud, e := a.cloudFor(c, item.Region, item.ResourceType)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	groups, e := cloud.Groups(r.Context())
	item.TestedAt = now()
	item.Status = "connected"
	label := "安全组"
	if resourceType(item.ResourceType) == "lighthouse" {
		label = "Lighthouse 实例"
	}
	detail := "连接正常，可见 " + strconv.Itoa(len(groups)) + " 个" + label + "（仅验证读取权限）"
	if e != nil {
		item.Status = "failed"
		detail = e.Error()
	}
	if saveErr := a.store.saveAccount(item, nil); saveErr != nil {
		fail(w, 500, "保存连接状态失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "account", Trigger: "manual", Object: item.Name, Detail: detail, Success: e == nil})
	if e != nil {
		fail(w, 502, detail)
		return
	}
	jsonResponse(w, 200, map[string]any{"count": len(groups), "message": detail})
}
func (a *App) groups(w http.ResponseWriter, r *http.Request) {
	item, e := a.store.account(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "云账号不存在")
		return
	}
	region := r.URL.Query().Get("region")
	if region == "" {
		region = item.Region
	}
	if !regionPattern.MatchString(region) {
		fail(w, 400, "地域格式无效")
		return
	}
	c, e := a.store.credentials(item.ID)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	kind := r.URL.Query().Get("resourceType")
	if kind == "" {
		kind = item.ResourceType
	}
	cloud, e := a.cloudFor(c, region, kind)
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	groups, e := cloud.Groups(r.Context())
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	jsonResponse(w, 200, groups)
}
func (a *App) saveTarget(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string    `json:"name"`
		ResourceType string    `json:"resourceType"`
		AccountID    string    `json:"accountId"`
		Region       string    `json:"region"`
		GroupID      string    `json:"groupId"`
		GroupName    string    `json:"groupName"`
		Ports        []Port    `json:"ports"`
		IPMode       string    `json:"ipMode"`
		FixedCIDR    string    `json:"fixedCidr"`
		FixedIPs     []FixedIP `json:"fixedIps"`
		AutoSync     bool      `json:"autoSync"`
		Note         string    `json:"note"`
	}
	if !decode(w, r, &input) {
		return
	}
	t := Target{Name: input.Name, ResourceType: input.ResourceType, AccountID: input.AccountID, Region: input.Region, GroupID: input.GroupID, GroupName: input.GroupName, Ports: input.Ports, IPMode: input.IPMode, FixedCIDR: input.FixedCIDR, FixedIPs: input.FixedIPs, AutoSync: input.AutoSync, Note: input.Note}
	if e := validateTarget(&t); e != nil {
		fail(w, 400, e.Error())
		return
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	if _, e := a.store.account(t.AccountID); e != nil {
		fail(w, 400, "云账号不存在")
		return
	}
	id := r.PathValue("id")
	if id != "" {
		old, e := a.store.target(id)
		if e != nil {
			fail(w, 404, "放行目标不存在")
			return
		}
		if old.AccountID != t.AccountID || old.Region != t.Region || old.GroupID != t.GroupID || resourceType(old.ResourceType) != t.ResourceType {
			fail(w, 400, "已有目标不能更换账号、地域或安全组，请创建新目标")
			return
		}
		t.ID = id
		if old.IPMode == t.IPMode {
			t.CurrentCIDR = old.CurrentCIDR
		}
		t.CurrentCIDRs = old.CurrentCIDRs
		if t.CurrentCIDRs == nil && old.CurrentCIDR != "" {
			t.CurrentCIDRs = []string{old.CurrentCIDR}
		}
		t.LastSync = old.LastSync
	} else {
		t.ID = randomID()
	}
	t.Status = "pending"
	if e := a.store.saveTarget(t); e != nil {
		fail(w, 500, "保存目标失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "target", Trigger: "manual", Object: t.GroupID, Detail: "保存放行目标", Success: true})
	ip := ""
	if t.IPMode == "dynamic" {
		ip, _ = a.cachedPublicIP()
	}
	result, e := a.syncTarget(r.Context(), t, ip, "manual", false)
	if e != nil {
		result.Status = "failed"
		result.Error = e.Error()
		_ = a.store.saveTarget(result)
	}
	jsonResponse(w, 200, result)
}
func (a *App) syncOne(w http.ResponseWriter, r *http.Request) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	t, e := a.store.target(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "放行目标不存在")
		return
	}
	ip := ""
	if t.IPMode == "dynamic" {
		ip, _ = a.ipForSync(r.Context(), "manual")
	}
	t, e = a.syncTarget(r.Context(), t, ip, "manual", false)
	if e != nil {
		fail(w, 502, e.Error())
		return
	}
	jsonResponse(w, 200, t)
}

func (a *App) toggleTarget(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &input) {
		return
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	t, e := a.store.target(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "放行目标不存在")
		return
	}
	t.AutoSync = input.Enabled
	if e = a.store.saveTarget(t); e != nil {
		fail(w, 500, "保存目标失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "target", Trigger: "manual", Object: t.GroupID, Detail: fmt.Sprintf("自动同步：%t", t.AutoSync), Success: true})
	jsonResponse(w, 200, t)
}
func (a *App) syncAll(w http.ResponseWriter, r *http.Request) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	targets, e := a.store.targets()
	if e != nil {
		fail(w, 500, "读取目标失败")
		return
	}
	needsIP := false
	for _, t := range targets {
		needsIP = needsIP || t.IPMode == "dynamic"
	}
	ip := ""
	if needsIP {
		ip, _ = a.ipForSync(r.Context(), "manual")
	}
	results := map[string]string{}
	success := 0
	for _, t := range targets {
		if _, e := a.syncTarget(r.Context(), t, ip, "manual", false); e != nil {
			results[t.ID] = e.Error()
		} else {
			success++
		}
	}
	jsonResponse(w, 200, map[string]any{"success": success, "failed": len(results), "errors": results})
}
func (a *App) deleteTarget(w http.ResponseWriter, r *http.Request) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	t, e := a.store.target(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "放行目标不存在")
		return
	}
	if r.URL.Query().Get("revoke") == "true" {
		if _, e = a.syncTarget(r.Context(), t, "", "manual", true); e != nil {
			fail(w, 502, e.Error())
			return
		}
	}
	if _, e = a.store.db.Exec("DELETE FROM targets WHERE id=?", t.ID); e != nil {
		fail(w, 500, "删除目标失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "target", Trigger: "manual", Object: t.GroupID, Detail: "删除放行目标；撤销云端规则：" + r.URL.Query().Get("revoke"), Success: true})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
func (a *App) deleteAccount(w http.ResponseWriter, r *http.Request) {
	a.mutate.Lock()
	defer a.mutate.Unlock()
	item, e := a.store.account(r.PathValue("id"))
	if e != nil {
		fail(w, 404, "云账号不存在")
		return
	}
	targets, e := a.store.targets()
	if e != nil {
		fail(w, 500, "读取目标失败")
		return
	}
	if r.URL.Query().Get("revoke") == "true" {
		for _, t := range targets {
			if t.AccountID == item.ID {
				if _, e = a.syncTarget(r.Context(), t, "", "manual", true); e != nil {
					fail(w, 502, "撤销未全部成功，账号和目标已保留："+e.Error())
					return
				}
			}
		}
	}
	tx, e := a.store.db.Begin()
	if e != nil {
		fail(w, 500, "删除账号失败")
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("DELETE FROM targets WHERE account_id=?", item.ID); e == nil {
		_, e = tx.Exec("DELETE FROM accounts WHERE id=?", item.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		fail(w, 500, "删除账号失败")
		return
	}
	_ = a.store.addLog(LogEntry{Kind: "account", Trigger: "manual", Object: item.Name, Detail: "删除账号及其目标；撤销云端规则：" + r.URL.Query().Get("revoke"), Success: true})
	jsonResponse(w, 200, map[string]bool{"ok": true})
}

type logFilter struct {
	where string
	args  []any
}

func filters(r *http.Request) logFilter {
	f := logFilter{where: " WHERE 1=1"}
	q := r.URL.Query()
	for _, field := range []string{"kind", "trigger"} {
		if v := q.Get(field); v != "" {
			f.where += " AND " + field + "=?"
			f.args = append(f.args, v)
		}
	}
	if v := q.Get("result"); v == "success" || v == "failed" {
		f.where += " AND success=?"
		f.args = append(f.args, v == "success")
	}
	if v := q.Get("q"); v != "" {
		f.where += " AND (object LIKE ? OR detail LIKE ?)"
		f.args = append(f.args, "%"+v+"%", "%"+v+"%")
	}
	days, _ := strconv.Atoi(q.Get("days"))
	if days != 7 && days != 30 && days != 90 {
		days = 7
	}
	f.where += " AND time>=?"
	f.args = append(f.args, time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339))
	return f
}
func readLogs(rows *sql.Rows) ([]LogEntry, error) {
	defer rows.Close()
	out := []LogEntry{}
	for rows.Next() {
		var l LogEntry
		var raw string
		if e := rows.Scan(&l.ID, &l.Time, &l.Kind, &l.Trigger, &l.Object, &l.Detail, &l.Success, &raw); e != nil {
			return nil, e
		}
		if e := json.Unmarshal([]byte(raw), &l.Changes); e != nil {
			return nil, e
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
func (a *App) logs(w http.ResponseWriter, r *http.Request) {
	f := filters(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	var total int
	if e := a.store.db.QueryRow("SELECT count(*) FROM logs"+f.where, f.args...).Scan(&total); e != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	args := append(f.args, 20, (page-1)*20)
	rows, e := a.store.db.Query("SELECT id,time,kind,trigger,object,detail,success,changes FROM logs"+f.where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if e != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	items, e := readLogs(rows)
	if e != nil {
		fail(w, 500, "读取日志失败")
		return
	}
	jsonResponse(w, 200, map[string]any{"items": items, "total": total, "page": page})
}
func csvSafe(s string) string {
	trim := strings.TrimLeft(s, " \t\r\n")
	if trim != "" && strings.ContainsAny(trim[:1], "=+-@") {
		return "'" + s
	}
	return s
}
func (a *App) exportLogs(w http.ResponseWriter, r *http.Request) {
	f := filters(r)
	rows, e := a.store.db.Query("SELECT id,time,kind,trigger,object,detail,success,changes FROM logs"+f.where+" ORDER BY id DESC LIMIT 10000", f.args...)
	if e != nil {
		fail(w, 500, "导出失败")
		return
	}
	items, e := readLogs(rows)
	if e != nil {
		fail(w, 500, "导出失败")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="gatesync-logs.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	c := csv.NewWriter(w)
	_ = c.Write([]string{"时间", "类型", "触发", "对象", "详情", "成功", "变更"})
	for _, l := range items {
		_ = c.Write([]string{l.Time, l.Kind, l.Trigger, csvSafe(l.Object), csvSafe(l.Detail), strconv.FormatBool(l.Success), csvSafe(strings.Join(l.Changes, "\n"))})
	}
	c.Flush()
}
