package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func ipLogCount(t *testing.T, a *App) int {
	t.Helper()
	var n int
	if e := a.store.db.QueryRow("SELECT COUNT(*) FROM logs WHERE kind='ip'").Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func setQuerySettings(t *testing.T, a *App, manual, scheduled bool) {
	t.Helper()
	s, e := a.settings()
	if e != nil {
		t.Fatal(e)
	}
	s.DetectOnManualSync, s.DetectOnSchedule = &manual, &scheduled
	// 私网地址在拨号前被现有检测器拒绝，无需访问外部服务即可确认是否触发检测。
	s.Sources = []string{"https://127.0.0.1"}
	if e := a.store.setMeta("settings", s); e != nil {
		t.Fatal(e)
	}
}

func setCachedIP(t *testing.T, a *App) {
	t.Helper()
	if e := a.store.setMeta("ip", IPState{IP: "8.8.8.8", Source: "https://example.com", CheckedAt: "2026-11-09T00:00:00Z"}); e != nil {
		t.Fatal(e)
	}
}

func TestIPQueryTriggersAreIndependent(t *testing.T) {
	a, _ := testApp(t)
	defaults, e := a.settings()
	if e != nil || !*defaults.DetectOnManualSync || !*defaults.DetectOnSchedule {
		t.Fatal("旧配置未获得兼容默认值", e)
	}
	for _, manual := range []bool{false, true} {
		for _, scheduled := range []bool{false, true} {
			t.Run(fmt.Sprintf("manual=%t/scheduled=%t", manual, scheduled), func(t *testing.T) {
				setQuerySettings(t, a, manual, scheduled)
				for _, trigger := range []string{"manual", "auto"} {
					setCachedIP(t, a)
					before := ipLogCount(t, a)
					ip, e := a.ipForSync(context.Background(), trigger)
					query := manual
					if trigger == "auto" {
						query = scheduled
					}
					if query {
						if e == nil || ip != "" || ipLogCount(t, a) != before+1 {
							t.Fatal("未触发检测或错误使用失败后的缓存", ip, e)
						}
					} else if e != nil || ip != "8.8.8.8" || ipLogCount(t, a) != before {
						t.Fatal("未按设置使用缓存", ip, e)
					}
				}
			})
		}
	}
	if _, e := a.cachedPublicIP(); e == nil {
		t.Fatal("检测失败后仍把缓存视为可用")
	}
}

func TestSavingTargetAlwaysUsesCache(t *testing.T) {
	a, cloud := testApp(t)
	target := sourceTarget(t, a)
	setQuerySettings(t, a, true, true)
	setCachedIP(t, a)
	before := ipLogCount(t, a)
	cookie := loginCookie(t, a)
	input := map[string]any{"accountId": target.AccountID, "region": target.Region, "groupId": target.GroupID, "ipMode": "dynamic", "ports": target.Ports, "fixedIps": []FixedIP{}, "note": "缓存目标"}
	w := request(t, a, "POST", "/api/targets", input, cookie)
	var result Target
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || w.Code != 200 || result.Status != "synced" || result.CurrentCIDR != "8.8.8.8/32" || ipLogCount(t, a) != before || len(cloud.rules) != 2 {
		t.Fatal("保存目标进行了检测或未使用缓存", e, w.Body.String())
	}
}

func TestOneQueryPerSyncRoundAndNoQueryForStaticTargets(t *testing.T) {
	for _, trigger := range []string{"manual", "auto"} {
		t.Run(trigger, func(t *testing.T) {
			a, _ := testApp(t)
			setQuerySettings(t, a, true, true)
			for i := 0; i < 3; i++ {
				target := sourceTarget(t, a)
				target.AutoSync = true
				if e := a.store.saveTarget(target); e != nil {
					t.Fatal(e)
				}
			}
			before := ipLogCount(t, a)
			if trigger == "auto" {
				a.syncScheduled(context.Background())
			} else if w := request(t, a, "POST", "/api/sync", nil, loginCookie(t, a)); w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			if ipLogCount(t, a) != before+1 {
				t.Fatal("同一轮同步重复查询了公网 IP")
			}
			if _, e := a.store.db.Exec("DELETE FROM targets"); e != nil {
				t.Fatal(e)
			}
			fixed := sourceTarget(t, a)
			fixed.IPMode = "fixed"
			fixed.FixedIPs = []FixedIP{{CIDR: "9.9.9.9/32"}}
			fixed.AutoSync = true
			if e := a.store.saveTarget(fixed); e != nil {
				t.Fatal(e)
			}
			before = ipLogCount(t, a)
			if trigger == "auto" {
				a.syncScheduled(context.Background())
			} else {
				request(t, a, "POST", "/api/sync", nil, loginCookie(t, a))
			}
			if ipLogCount(t, a) != before {
				t.Fatal("纯固定 IP 目标触发了公网 IP 查询")
			}
		})
	}
}

func TestQuerySettingsPersistAndManualSyncUsesCache(t *testing.T) {
	a, cloud := testApp(t)
	cookie := loginCookie(t, a)
	s, e := a.settings()
	if e != nil {
		t.Fatal(e)
	}
	manual, scheduled := false, true
	s.DetectOnManualSync, s.DetectOnSchedule = &manual, &scheduled
	s.Sources = []string{"https://127.0.0.1"}
	if w := request(t, a, "PUT", "/api/settings", s, cookie); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	saved, e := a.settings()
	if e != nil || *saved.DetectOnManualSync || !*saved.DetectOnSchedule {
		t.Fatal("查询设置未持久化", e)
	}
	setCachedIP(t, a)
	target := sourceTarget(t, a)
	if e := a.store.saveTarget(target); e != nil {
		t.Fatal(e)
	}
	if w := request(t, a, "POST", "/api/targets/"+target.ID+"/sync", nil, cookie); w.Code != 200 || len(cloud.rules) != 2 || ipLogCount(t, a) != 0 {
		t.Fatal("手动同步未尊重缓存设置", w.Body.String())
	}
}
