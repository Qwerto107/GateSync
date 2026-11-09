package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestTargetOrderPersistsWithoutCloudChanges(t *testing.T) {
	a, cloud := testApp(t)
	cookie := loginCookie(t, a)
	first := sourceTarget(t, a)
	first.Name, first.Status, first.Error = "办公室", "failed", "保留错误"
	second := first
	second.ID, second.Name = randomID(), "备用网络"
	for _, target := range []Target{first, second} {
		if e := a.store.saveTarget(target); e != nil {
			t.Fatal(e)
		}
	}
	check := func(ids []string) {
		t.Helper()
		targets, e := a.store.targets()
		if e != nil {
			t.Fatal(e)
		}
		actual := []string{}
		for _, target := range targets {
			actual = append(actual, target.ID)
		}
		if !slices.Equal(actual, ids) {
			t.Fatal("目标顺序错误", actual, ids)
		}
	}
	check([]string{first.ID, second.ID})
	order := []string{second.ID, first.ID}
	if w := request(t, a, "PUT", "/api/targets/order", map[string]any{"ids": order}, nil); w.Code != 401 {
		t.Fatal("排序接口缺少登录限制", w.Code)
	}
	if w := request(t, a, "PUT", "/api/targets/order", map[string]any{"ids": order}, cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	check(order)
	var persisted []string
	if e := a.store.getMeta("target-order", &persisted); e != nil || !slices.Equal(persisted, order) {
		t.Fatal("排序未写入 SQLite", persisted, e)
	}
	for _, invalid := range [][]string{{first.ID}, {first.ID, first.ID}, {first.ID, "unknown"}} {
		if w := request(t, a, "PUT", "/api/targets/order", map[string]any{"ids": invalid}, cookie); w.Code != 409 {
			t.Fatal("未拒绝过期或重复顺序", w.Code)
		}
		check(order)
	}
	saved, e := a.store.target(first.ID)
	before, _ := json.Marshal(first)
	after, _ := json.Marshal(saved)
	if e != nil || string(before) != string(after) || len(cloud.calls) != 0 {
		t.Fatal("排序修改了目标配置或调用云端", e)
	}
	third := first
	third.ID = randomID()
	if e := a.store.saveTarget(third); e != nil {
		t.Fatal(e)
	}
	check([]string{second.ID, first.ID, third.ID})
	if _, e := a.store.db.Exec("DELETE FROM targets WHERE id=?", second.ID); e != nil {
		t.Fatal(e)
	}
	check([]string{first.ID, third.ID})
}

func TestTargetNameSavedSeparatelyFromCloudRemark(t *testing.T) {
	a, cloud := testApp(t)
	target := sourceTarget(t, a)
	target.Name = "  办公室 SSH  "
	target.IPMode = "fixed"
	target.FixedIPs = []FixedIP{{CIDR: "8.8.8.8"}}
	input := map[string]any{"name": target.Name, "accountId": target.AccountID, "region": target.Region, "groupId": target.GroupID, "ipMode": target.IPMode, "ports": target.Ports, "fixedIps": target.FixedIPs, "note": target.Note}
	w := request(t, a, "POST", "/api/targets", input, loginCookie(t, a))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var saved Target
	if e := json.Unmarshal(w.Body.Bytes(), &saved); e != nil || saved.Name != "办公室 SSH" || saved.Note != target.Note {
		t.Fatal("名称或备注保存错误", e, saved.Name, saved.Note)
	}
	for _, rule := range cloud.rules {
		if !strings.HasPrefix(rule.Description, target.Note+" [GS:") {
			t.Fatal("目标名称替代了云端备注", rule.Description)
		}
	}
	target.Name = strings.Repeat("名", 61)
	if e := validateTarget(&target); e == nil {
		t.Fatal("未限制名称长度")
	}
}
