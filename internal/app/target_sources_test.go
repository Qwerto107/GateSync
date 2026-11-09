package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func sourceTarget(t *testing.T, a *App) Target {
	t.Helper()
	account := Account{ID: randomID(), Name: "测试", Region: "ap-guangzhou"}
	if e := a.store.saveAccount(account, &Credentials{SecretID: "test", SecretKey: "test"}); e != nil {
		t.Fatal(e)
	}
	return Target{ID: randomID(), AccountID: account.ID, Region: account.Region, GroupID: "sg-test", IPMode: "dynamic", Ports: []Port{{"TCP", "22"}, {"UDP", "53"}}, Note: "办公室 SSH"}
}

func TestTargetMultipleSourcesSharePortsAndKeepFixedIP(t *testing.T) {
	a, cloud := testApp(t)
	target := sourceTarget(t, a)
	target.FixedIPs = []FixedIP{{CIDR: "1.1.1.1", Name: "家里"}, {CIDR: "1.1.1.1/32"}, {CIDR: "9.9.9.9"}}
	if e := validateTarget(&target); e != nil {
		t.Fatal(e)
	}
	if len(target.FixedIPs) != 2 {
		t.Fatal("固定 IP 未规范化去重")
	}
	result, e := a.syncTarget(context.Background(), target, "1.1.1.1", "manual", false)
	if e != nil || len(cloud.rules) != 4 || len(result.CurrentCIDRs) != 2 {
		t.Fatal(e, cloud.rules, result)
	}
	// 动态来源与固定地址重合时，切换出口后仍须保留原地址的两条端口规则。
	result, e = a.syncTarget(context.Background(), result, "8.8.8.8", "manual", false)
	if e != nil || len(cloud.rules) != 6 {
		t.Fatal(e, cloud.rules)
	}
	if slices.Contains(cloud.calls, "delete") {
		t.Fatal("误删与旧动态地址重合的固定 IP")
	}
	result, e = a.syncTarget(context.Background(), result, "4.4.4.4", "manual", false)
	if e != nil || len(cloud.rules) != 6 || slices.ContainsFunc(cloud.rules, func(r Rule) bool { return r.CIDR == "8.8.8.8/32" }) {
		t.Fatal(e, cloud.rules)
	}
	for _, r := range cloud.rules {
		if !strings.HasPrefix(r.Description, "办公室 SSH [GS:") {
			t.Fatal(r.Description)
		}
	}
	if _, e = a.syncTarget(context.Background(), result, "", "manual", true); e != nil || len(cloud.rules) != 0 {
		t.Fatal("多 IP 撤销失败", e)
	}
}

func TestDetectionFailureAddsFixedSourcesWithoutDeletingOldRules(t *testing.T) {
	a, cloud := testApp(t)
	target := sourceTarget(t, a)
	target.FixedIPs = []FixedIP{{CIDR: "9.9.9.9/32"}}
	result, e := a.syncTarget(context.Background(), target, "1.1.1.1", "manual", false)
	if e != nil {
		t.Fatal(e)
	}
	result.FixedIPs = []FixedIP{{CIDR: "4.4.4.4/32"}}
	before := len(cloud.calls)
	result, e = a.syncTarget(context.Background(), result, "", "manual", false)
	if e == nil || !strings.Contains(e.Error(), "固定 IP 已检查") || result.Status != "failed" || len(cloud.rules) != 6 || result.CurrentCIDR != "1.1.1.1/32" {
		t.Fatal(e, result, cloud.rules)
	}
	if slices.Contains(cloud.calls[before:], "delete") {
		t.Fatal("检测失败时删除了旧规则")
	}
	result, e = a.syncTarget(context.Background(), result, "8.8.8.8", "manual", false)
	if e != nil || len(cloud.rules) != 4 || len(result.CurrentCIDRs) != 2 {
		t.Fatal("恢复检测后未收敛", e, result, cloud.rules)
	}
}

func TestDescriptionMigrationRenameAndLostResponse(t *testing.T) {
	legacy := "gatesync:installation:target"
	marker := shortMarker(legacy)
	old := testRule("1.1.1.1/32", legacy)
	foreign := testRule("9.9.9.9/32", "人工创建 "+marker)
	cloud := &fakeCloud{version: 1, rules: []Rule{old, foreign}, loseRemarkResponse: true}
	ledger := []Rule{old}
	save := func(r []Rule) error { ledger = slices.Clone(r); return nil }
	want := []Rule{testRule(old.CIDR, cloudDescription("办公室 SSH", marker))}
	if _, e := reconcile(context.Background(), cloud, "sg-test", marker, want, ledger, save); e == nil {
		t.Fatal("未模拟备注响应丢失")
	}
	if len(ledger) != 2 {
		t.Fatal("备注修改前没有保留两种已知描述")
	}
	if _, e := reconcile(context.Background(), cloud, "sg-test", marker, want, ledger, save); e != nil {
		t.Fatal(e)
	}
	want[0].Description = cloudDescription("新的备注", marker)
	if _, e := reconcile(context.Background(), cloud, "sg-test", marker, want, ledger, save); e != nil {
		t.Fatal(e)
	}
	if cloud.rules[0].Description != want[0].Description || cloud.rules[1].Description != foreign.Description || slices.Contains(cloud.calls, "add") || slices.Contains(cloud.calls, "delete") {
		t.Fatal("迁移或改名造成规则重建或触碰其他规则", cloud.rules, cloud.calls)
	}
	long := cloudDescription(strings.Repeat("中文备注", 30), marker)
	if len(long) > 64 || !utf8.ValidString(long) || !strings.HasSuffix(long, marker) {
		t.Fatal("备注截断破坏标记或 UTF-8", long)
	}
}

func TestDescriptionFailureNeverDeletesOldIP(t *testing.T) {
	marker := shortMarker("gatesync:test:target")
	old := testRule("1.1.1.1/32", cloudDescription("旧备注", marker))
	keep := testRule("9.9.9.9/32", old.Description)
	cloud := &fakeCloud{version: 1, rules: []Rule{old, keep}, failRemark: true}
	want := []Rule{testRule("8.8.8.8/32", cloudDescription("新备注", marker)), testRule(keep.CIDR, cloudDescription("新备注", marker))}
	if _, e := reconcile(context.Background(), cloud, "sg-test", marker, want, []Rule{old, keep}, func([]Rule) error { return nil }); e == nil || slices.Contains(cloud.calls, "delete") {
		t.Fatal("备注失败后删除了旧 IP", e, cloud.calls)
	}
}

func TestLegacySourceAndStateCompatibility(t *testing.T) {
	a, _ := testApp(t)
	target := sourceTarget(t, a)
	target.IPMode, target.FixedCIDR = "fixed", "8.8.8.8"
	if e := validateTarget(&target); e != nil || len(target.FixedIPs) != 1 || target.FixedIPs[0].CIDR != "8.8.8.8/32" {
		t.Fatal(e, target)
	}
	target.IPMode = "dynamic"
	target.CurrentCIDR = "1.1.1.1/32"
	target.CurrentCIDRs = []string{"1.1.1.1/32", "8.8.8.8/32"}
	target.Status = "synced"
	if e := a.store.saveTarget(target); e != nil {
		t.Fatal(e)
	}
	if e := a.store.setMeta("ip", IPState{IP: "9.9.9.9"}); e != nil {
		t.Fatal(e)
	}
	w := request(t, a, "GET", "/api/state", nil, loginCookie(t, a))
	var state struct {
		Targets []Target `json:"targets"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &state); e != nil || len(state.Targets) != 1 || state.Targets[0].Status != "outdated" {
		t.Fatal(e, w.Body.String())
	}
	target.FixedIPs = []FixedIP{}
	target.IPMode = "fixed"
	if validateTarget(&target) == nil {
		t.Fatal("允许未选择任何 IP 来源")
	}
}
