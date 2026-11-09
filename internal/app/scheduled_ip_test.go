package app

import (
	"context"
	"testing"
)

func TestScheduledSyncOnlyOnIPChange(t *testing.T) {
	a, cloud := testApp(t)
	setQuerySettings(t, a, false, false)
	setCachedIP(t, a)
	target := sourceTarget(t, a)
	target.AutoSync = true
	if _, e := a.syncTarget(context.Background(), target, "8.8.8.8", "manual", false); e != nil {
		t.Fatal(e)
	}
	// 云端规则即使被手动改动，地址不变的定时任务也不做读取或修复。
	cloud.rules = nil
	cloud.calls = nil
	a.syncScheduled(context.Background())
	if len(cloud.calls) != 0 {
		t.Fatal("IP 未变时访问了云端", cloud.calls)
	}
	if e := a.store.setMeta("ip", IPState{IP: "9.9.9.9"}); e != nil {
		t.Fatal(e)
	}
	a.syncScheduled(context.Background())
	if len(cloud.rules) != 2 || len(cloud.calls) == 0 {
		t.Fatal("IP 变化时未同步", cloud.rules, cloud.calls)
	}
	cloud.calls = nil
	a.syncScheduled(context.Background())
	if len(cloud.calls) != 0 {
		t.Fatal("相同 IP 重复同步", cloud.calls)
	}
	// 变更同步失败只尝试一次，不因后续检测到同一 IP 而重复核对规则。
	cloud.failAdd = true
	if e := a.store.setMeta("ip", IPState{IP: "4.4.4.4"}); e != nil {
		t.Fatal(e)
	}
	a.syncScheduled(context.Background())
	saved, e := a.store.target(target.ID)
	if e != nil || saved.Status != "failed" || saved.CurrentCIDR != "9.9.9.9/32" || len(cloud.rules) != 2 {
		t.Fatal("失败时未保留旧规则和错误状态", saved, e)
	}
	var processed string
	if e := a.store.getMeta("scheduled-ip:"+target.ID, &processed); e != nil || processed != "4.4.4.4" {
		t.Fatal("处理地址未持久化", processed, e)
	}
	cloud.calls = nil
	a.syncScheduled(context.Background())
	if len(cloud.calls) != 0 {
		t.Fatal("失败后重复调用云端", cloud.calls)
	}
	cloud.failAdd = false
	if w := request(t, a, "POST", "/api/targets/"+target.ID+"/sync", nil, loginCookie(t, a)); w.Code != 200 || len(cloud.calls) == 0 {
		t.Fatal("手动同步无法重试", w.Code, w.Body.String())
	}
	cloud.calls = nil
	if e := a.store.setMeta("ip", IPState{IP: "1.1.1.1", Error: "检测失败"}); e != nil {
		t.Fatal(e)
	}
	a.syncScheduled(context.Background())
	if len(cloud.calls) != 0 {
		t.Fatal("检测失败时访问了云端", cloud.calls)
	}
}

func TestScheduledBaselineSkipsFixedAndDisabledTargets(t *testing.T) {
	a, cloud := testApp(t)
	setQuerySettings(t, a, false, false)
	setCachedIP(t, a)
	dynamic := sourceTarget(t, a)
	dynamic.AutoSync = true
	fixed := dynamic
	fixed.ID, fixed.IPMode = randomID(), "fixed"
	fixed.FixedIPs = []FixedIP{{CIDR: "1.1.1.1/32"}}
	disabled := dynamic
	disabled.ID, disabled.AutoSync = randomID(), false
	for _, target := range []Target{dynamic, fixed, disabled} {
		if e := a.store.saveTarget(target); e != nil {
			t.Fatal(e)
		}
	}
	a.syncScheduled(context.Background())
	if len(cloud.calls) != 0 {
		t.Fatal("建立基线时访问了云端")
	}
	if e := a.store.setMeta("ip", IPState{IP: "9.9.9.9"}); e != nil {
		t.Fatal(e)
	}
	a.syncScheduled(context.Background())
	if len(cloud.rules) != 2 {
		t.Fatal("动态目标未同步，或固定/停用目标参与了同步", cloud.rules)
	}
	for _, target := range []Target{fixed, disabled} {
		saved, e := a.store.target(target.ID)
		if e != nil || saved.LastSync != "" {
			t.Fatal("固定或停用目标被自动同步", saved, e)
		}
	}
}
