package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func publicIPv4(ip netip.Addr) bool {
	if !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return false
		}
	}
	return true
}
func validateSource(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return fmt.Errorf("IP 检测源必须为公开的 HTTPS 地址（443 端口）")
	}
	return nil
}
func ipHTTPClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{
		// 在实际拨号时校验解析结果，阻止自定义检测源访问内网及 DNS 重绑定。
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, e := net.SplitHostPort(address)
			if e != nil {
				return nil, e
			}
			ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
			if e != nil {
				return nil, e
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("检测源没有 IPv4 地址")
			}
			for _, ip := range ips {
				if !publicIPv4(ip) {
					return nil, fmt.Errorf("检测源不能指向内网或保留地址")
				}
			}
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", net.JoinHostPort(ips[0].String(), port))
		}, ResponseHeaderTimeout: 5 * time.Second, MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second,
	}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("重定向过多")
		}
		return validateSource(req.URL.String())
	}}
}
func (a *App) detectIP(ctx context.Context, trigger string) (IPState, error) {
	a.detectMu.Lock()
	defer a.detectMu.Unlock()
	s, e := a.settings()
	if e != nil {
		return IPState{}, e
	}
	var old IPState
	_ = a.store.getMeta("ip", &old)
	client := ipHTTPClient()
	defer client.CloseIdleConnections()
	for _, source := range s.Sources {
		if validateSource(source) != nil {
			continue
		}
		req, e := http.NewRequestWithContext(ctx, "GET", source, nil)
		if e != nil {
			continue
		}
		req.Header.Set("Accept", "text/plain")
		res, e := client.Do(req)
		if e != nil {
			continue
		}
		b, readErr := io.ReadAll(io.LimitReader(res.Body, 129))
		res.Body.Close()
		if readErr != nil || res.StatusCode != 200 || len(b) > 128 {
			continue
		}
		ip, e := netip.ParseAddr(strings.TrimSpace(string(b)))
		if e != nil || !publicIPv4(ip) {
			continue
		}
		state := IPState{IP: ip.String(), Source: source, CheckedAt: now()}
		if e = a.store.setMeta("ip", state); e != nil {
			return old, e
		}
		detail := "公网 IP 无变化：" + state.IP
		if old.IP != state.IP {
			detail = old.IP + " → " + state.IP
		}
		_ = a.store.addLog(LogEntry{Kind: "ip", Trigger: trigger, Object: "公网 IP", Detail: detail, Success: true})
		return state, nil
	}
	old.Error = "所有 IP 检测源均不可用，已保留旧 IP，暂停本次动态规则同步"
	_ = a.store.setMeta("ip", old)
	_ = a.store.addLog(LogEntry{Kind: "ip", Trigger: trigger, Object: "公网 IP", Detail: old.Error, Success: false})
	return old, errors.New(old.Error)
}
func (a *App) cachedPublicIP() (string, error) {
	var cached IPState
	if e := a.store.getMeta("ip", &cached); e != nil {
		return "", fmt.Errorf("尚无公网 IP 缓存，请先点击重新检测")
	}
	ip, e := netip.ParseAddr(cached.IP)
	if e != nil || !publicIPv4(ip) || cached.Error != "" {
		return "", fmt.Errorf("公网 IP 缓存不可用，请重新检测")
	}
	return ip.String(), nil
}
func (a *App) ipForSync(ctx context.Context, trigger string) (string, error) {
	settings, e := a.settings()
	if e != nil {
		return "", e
	}
	query := *settings.DetectOnManualSync
	if trigger == "auto" {
		query = *settings.DetectOnSchedule
	}
	if !query {
		return a.cachedPublicIP()
	}
	detected, e := a.detectIP(ctx, trigger)
	// 检测失败不可回退到旧缓存继续清理，交给同步逻辑保留已有规则。
	if e != nil {
		return "", e
	}
	return detected.IP, nil
}

func ruleKey(r Rule) string { return strings.ToUpper(r.Protocol) + "|" + r.Port + "|" + r.CIDR }
func shortMarker(legacy string) string {
	sum := sha256.Sum256([]byte(legacy))
	return "[GS:" + base64.RawURLEncoding.EncodeToString(sum[:12]) + "]"
}
func cloudDescription(note, marker string) string {
	note = strings.Join(strings.Fields(note), " ")
	if note == "" {
		note = "GateSync"
	}
	// 按 UTF-8 字节保守截断可读部分，完整保留归属标记，适配 Lighthouse 长度限制。
	limit := 64 - len(marker) - 1
	out := ""
	for _, r := range note {
		if len(out)+len(string(r)) > limit {
			break
		}
		out += string(r)
	}
	return strings.TrimSpace(out) + " " + marker
}
func matchesMarker(description, marker string) bool {
	if description == marker || strings.HasSuffix(description, " "+marker) {
		return true
	}
	// 旧备注可由同一实例、目标推导出新标记，迁移后仍只认本地台账内的规则。
	return strings.HasPrefix(description, "gatesync:") && shortMarker(description) == marker
}
func ledgerKey(r Rule) string { return ruleKey(r) + "|" + r.Description }
func ownedRules(rules, ledger []Rule, marker string) []Rule {
	known := map[string]bool{}
	for _, r := range ledger {
		known[ledgerKey(r)] = true
	}
	out := []Rule{}
	for _, r := range rules {
		if matchesMarker(r.Description, marker) && known[ledgerKey(r)] && strings.EqualFold(r.Action, "ACCEPT") {
			out = append(out, r)
		}
	}
	return out
}
func reconcile(ctx context.Context, c Cloud, group, marker string, want, ledger []Rule, saveLedger func([]Rule) error, preserveStale ...bool) ([]string, error) {
	changes := []string{}
	set, e := c.Rules(ctx, group)
	if e != nil {
		return changes, e
	}
	record := func(rules []Rule) error {
		next := append([]Rule{}, ledger...)
		known := map[string]bool{}
		for _, r := range next {
			known[ledgerKey(r)] = true
		}
		for _, r := range rules {
			if !known[ledgerKey(r)] {
				next = append(next, r)
				known[ledgerKey(r)] = true
			}
		}
		if e := saveLedger(next); e != nil {
			return e
		}
		ledger = next
		return nil
	}
	current := ownedRules(set.Rules, ledger, marker)
	have := map[string]bool{}
	for _, r := range current {
		if have[ruleKey(r)] {
			return changes, fmt.Errorf("存在重复的归属规则，已停止同步")
		}
		have[ruleKey(r)] = true
	}
	missing := []Rule{}
	for _, r := range want {
		if !have[ruleKey(r)] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		if set.Version == "" {
			return changes, fmt.Errorf("缺少云端规则版本，已停止同步")
		}
		// 新增和备注更新都先保存意图；响应丢失时，下一次能识别两种已知描述。
		if e = record(missing); e != nil {
			return changes, e
		}
		if e = c.Add(ctx, group, set.Version, missing); e != nil {
			return changes, e
		}
		for _, r := range missing {
			changes = append(changes, "+ 新增 "+r.Protocol+" "+r.Port+" "+r.CIDR)
		}
	}
	set, e = c.Rules(ctx, group)
	if e != nil {
		return changes, e
	}
	// 先核验所有新规则，再原地更新备注；任何失败都会保留旧 IP 规则。
	current = ownedRules(set.Rules, ledger, marker)
	have = map[string]bool{}
	for _, r := range current {
		have[ruleKey(r)] = true
	}
	for _, wanted := range want {
		if !have[ruleKey(wanted)] {
			return changes, fmt.Errorf("新规则尚未全部确认，已保留旧规则")
		}
	}
	for _, wanted := range want {
		current = ownedRules(set.Rules, ledger, marker)
		for _, old := range current {
			if ruleKey(old) != ruleKey(wanted) || old.Description == wanted.Description {
				continue
			}
			if e = record([]Rule{wanted}); e != nil {
				return changes, e
			}
			if e = c.UpdateDescription(ctx, group, set.Version, old, wanted.Description); e != nil {
				return changes, fmt.Errorf("更新云端备注失败：%w", e)
			}
			set, e = c.Rules(ctx, group)
			if e != nil {
				return changes, e
			}
			confirmed := false
			for _, r := range ownedRules(set.Rules, ledger, marker) {
				if ledgerKey(r) == ledgerKey(wanted) {
					confirmed = true
				}
			}
			if !confirmed {
				return changes, fmt.Errorf("云端备注更新尚未确认，已保留旧规则")
			}
			changes = append(changes, "~ 更新备注 "+old.Protocol+" "+old.Port+" "+old.CIDR)
			break
		}
	}
	current = ownedRules(set.Rules, ledger, marker)
	desired := map[string]bool{}
	for _, r := range want {
		desired[ruleKey(r)] = true
	}
	stale := []Rule{}
	for _, r := range current {
		if !desired[ruleKey(r)] {
			stale = append(stale, r)
		}
	}
	if len(preserveStale) > 0 && preserveStale[0] {
		// 动态 IP 未知时仅补充固定规则，推迟清理，避免把旧动态授权当成过期规则删除。
		next := append([]Rule{}, want...)
		next = append(next, stale...)
		return changes, saveLedger(next)
	}
	if len(stale) > 0 {
		if e = c.Delete(ctx, group, set.Version, stale); e != nil {
			return changes, e
		}
		for _, r := range stale {
			changes = append(changes, "- 删除 "+r.Protocol+" "+r.Port+" "+r.CIDR)
		}
		set, e = c.Rules(ctx, group)
		if e != nil {
			return changes, e
		}
		for _, r := range ownedRules(set.Rules, ledger, marker) {
			if !desired[ruleKey(r)] {
				return changes, fmt.Errorf("旧规则撤销尚未确认，请重试")
			}
		}
	}
	if e = saveLedger(want); e != nil {
		return changes, e
	}
	return changes, nil
}
func (a *App) syncTarget(ctx context.Context, t Target, ip, trigger string, revoke bool) (result Target, err error) {
	result = t
	object := t.GroupID
	cidr := t.CurrentCIDR
	if t.IPMode == "dynamic" && ip != "" && !revoke {
		cidr = ip + "/32"
	}
	if t.IPMode == "fixed" {
		cidr = ""
		if sources := t.fixedIPs(); len(sources) > 0 {
			cidr = sources[0].CIDR
		}
	}
	preserve := t.IPMode == "dynamic" && ip == "" && !revoke
	var desired []Rule
	var changes []string
	defer func() {
		detail := t.CurrentCIDR + " → " + cidr
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			detail = err.Error()
		} else if revoke {
			// 部分账号撤销失败后保留本地记录，已撤销目标必须停用自动同步，避免重新开放。
			result.Status = "revoked"
			result.Error = ""
			result.CurrentCIDR = ""
			result.CurrentCIDRs = []string{}
			result.AutoSync = false
			result.LastSync = now()
			detail = "撤销本工具创建的规则，已暂停目标自动同步"
		} else {
			result.Status = "synced"
			result.Error = ""
			result.CurrentCIDR = cidr
			result.LastSync = now()
		}
		saveErr := a.store.saveTarget(result)
		logErr := a.store.addLog(LogEntry{Kind: "sync", Trigger: trigger, Object: object, Detail: detail, Success: err == nil, Changes: changes})
		if err == nil {
			err = errors.Join(saveErr, logErr)
		}
	}()
	account, e := a.store.account(t.AccountID)
	if e != nil {
		return t, e
	}
	object = account.Name + " / " + t.GroupID
	credentials, e := a.store.credentials(account.ID)
	if e != nil {
		return t, e
	}
	c, e := a.cloudFor(credentials, t.Region, t.ResourceType)
	if e != nil {
		return t, e
	}
	if preserve && len(t.fixedIPs()) == 0 {
		return t, fmt.Errorf("公网 IP 不可用（未检测或检测失败），已保留旧规则；请重新检测或检查查询设置")
	}
	marker := shortMarker("gatesync:" + a.installation + ":" + t.ID)
	if !revoke {
		for _, source := range t.desiredCIDRs(ip) {
			for _, p := range t.Ports {
				desired = append(desired, Rule{Protocol: p.Protocol, Port: p.Value, CIDR: source, Action: "ACCEPT", Description: cloudDescription(t.Note, marker)})
			}
		}
	}
	var ledger []Rule
	if e = a.store.getMeta("owned:"+t.ID, &ledger); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return t, e
	}
	changes, e = reconcile(ctx, c, t.GroupID, marker, desired, ledger, func(r []Rule) error {
		if e := a.store.setMeta("owned:"+t.ID, r); e != nil {
			return e
		}
		ledger = r
		return nil
	}, preserve)
	if e == nil && !revoke {
		result.CurrentCIDRs = []string{}
		seen := map[string]bool{}
		for _, r := range ledger {
			if !seen[r.CIDR] {
				result.CurrentCIDRs = append(result.CurrentCIDRs, r.CIDR)
				seen[r.CIDR] = true
			}
		}
		if preserve {
			e = fmt.Errorf("公网 IP 不可用，固定 IP 已检查，旧规则已保留；请重新检测或检查查询设置")
		}
	}
	return result, e
}
