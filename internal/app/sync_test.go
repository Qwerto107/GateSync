package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
)

type fakeCloud struct {
	rules                                    []Rule
	version                                  int
	calls                                    []string
	failAdd, failDelete, hideAdded, conflict bool
	failSecondDelete                         bool
	deleteCount                              int
	failRemark, loseRemarkResponse           bool
}

func (f *fakeCloud) Groups(context.Context) ([]Group, error) {
	return []Group{{ID: "sg-test", Name: "测试安全组"}}, nil
}
func (f *fakeCloud) Rules(context.Context, string) (RuleSet, error) {
	out := slices.Clone(f.rules)
	if f.hideAdded {
		out = slices.DeleteFunc(out, func(r Rule) bool { return r.CIDR == "8.8.8.8/32" })
	}
	for i := range out {
		out[i].Index = int64(i)
	}
	f.calls = append(f.calls, "read")
	return RuleSet{Version: fmt.Sprint(f.version), Rules: out}, nil
}
func (f *fakeCloud) Add(_ context.Context, _ string, version string, rules []Rule) error {
	f.calls = append(f.calls, "add")
	if f.failAdd {
		return errors.New("添加失败")
	}
	if version != fmt.Sprint(f.version) {
		return errors.New("版本冲突")
	}
	f.rules = append(f.rules, rules...)
	f.version++
	return nil
}
func (f *fakeCloud) Delete(_ context.Context, _ string, version string, rules []Rule) error {
	f.calls = append(f.calls, "delete")
	f.deleteCount++
	if f.failDelete || f.conflict || (f.failSecondDelete && f.deleteCount == 2) || version != fmt.Sprint(f.version) {
		return errors.New("删除失败或版本冲突")
	}
	indices := map[int64]bool{}
	for _, r := range rules {
		indices[r.Index] = true
	}
	out := []Rule{}
	for i, r := range f.rules {
		if !indices[int64(i)] {
			out = append(out, r)
		}
	}
	f.rules = out
	f.version++
	return nil
}
func (f *fakeCloud) UpdateDescription(_ context.Context, _ string, version string, old Rule, description string) error {
	f.calls = append(f.calls, "remark")
	if f.failRemark {
		return errors.New("无备注修改权限")
	}
	if f.conflict || version != fmt.Sprint(f.version) {
		return errors.New("版本冲突")
	}
	if old.Index < 0 || int(old.Index) >= len(f.rules) || ledgerKey(f.rules[old.Index]) != ledgerKey(old) {
		return errors.New("规则已变更")
	}
	f.rules[old.Index].Description = description
	f.version++
	if f.loseRemarkResponse {
		f.loseRemarkResponse = false
		return errors.New("备注响应丢失")
	}
	return nil
}
func testRule(cidr, description string) Rule {
	return Rule{Protocol: "TCP", Port: "22", CIDR: cidr, Action: "ACCEPT", Description: description}
}

func TestReconcileProtectsRulesAndOrdersMutations(t *testing.T) {
	marker := "gatesync:test:target"
	old := testRule("1.1.1.1/32", marker)
	want := testRule("8.8.8.8/32", marker)
	foreign := testRule("9.9.9.9/32", "人工创建")
	forged := testRule("4.4.4.4/32", marker)
	f := &fakeCloud{rules: []Rule{foreign, old, forged}, version: 2}
	ledger := []Rule{old}
	changes, e := reconcile(context.Background(), f, "sg-test", marker, []Rule{want}, ledger, func(r []Rule) error { ledger = slices.Clone(r); return nil })
	if e != nil {
		t.Fatal(e)
	}
	if len(changes) != 2 || len(f.rules) != 3 {
		t.Fatalf("unexpected changes/rules: %v %v", changes, f.rules)
	}
	if slices.Index(f.calls, "add") > slices.Index(f.calls, "delete") {
		t.Fatal("旧规则先于新规则被删除")
	}
	if !slices.ContainsFunc(f.rules, func(r Rule) bool { return r.CIDR == foreign.CIDR }) || !slices.ContainsFunc(f.rules, func(r Rule) bool { return r.CIDR == forged.CIDR }) {
		t.Fatal("非本工具台账规则被删除")
	}
	before := len(f.calls)
	_, e = reconcile(context.Background(), f, "sg-test", marker, []Rule{want}, ledger, func(r []Rule) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	for _, call := range f.calls[before:] {
		if call != "read" {
			t.Fatal("重复同步进行了变更")
		}
	}
}
func TestReconcileKeepsOldRulesOnFailure(t *testing.T) {
	for _, mode := range []string{"add", "verify", "delete", "conflict", "persist"} {
		t.Run(mode, func(t *testing.T) {
			marker := "gatesync:test:target"
			old := testRule("1.1.1.1/32", marker)
			want := testRule("8.8.8.8/32", marker)
			f := &fakeCloud{rules: []Rule{old}, version: 1, failAdd: mode == "add", hideAdded: mode == "verify", failDelete: mode == "delete", conflict: mode == "conflict"}
			_, e := reconcile(context.Background(), f, "sg-test", marker, []Rule{want}, []Rule{old}, func([]Rule) error {
				if mode == "persist" {
					return errors.New("磁盘已满")
				}
				return nil
			})
			if e == nil {
				t.Fatal("应返回失败")
			}
			if !slices.ContainsFunc(f.rules, func(r Rule) bool { return r.CIDR == old.CIDR }) {
				t.Fatal("失败时旧规则丢失")
			}
			if (mode == "add" || mode == "verify" || mode == "persist") && slices.Contains(f.calls, "delete") {
				t.Fatal("未确认新规则时尝试删除")
			}
			if mode == "persist" && slices.Contains(f.calls, "add") {
				t.Fatal("台账未保存就写入云端")
			}
		})
	}
}
func TestReconcileRecoversPersistedCreateIntent(t *testing.T) {
	marker := "gatesync:test:target"
	old := testRule("1.1.1.1/32", marker)
	want := testRule("8.8.8.8/32", marker)
	// 模拟创建成功但响应丢失，下一次同步只需核验和删除旧规则。
	f := &fakeCloud{rules: []Rule{old, want}, version: 3}
	_, e := reconcile(context.Background(), f, "sg-test", marker, []Rule{want}, []Rule{old, want}, func([]Rule) error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	if slices.Contains(f.calls, "add") || len(f.rules) != 1 || f.rules[0].CIDR != want.CIDR {
		t.Fatalf("恢复失败：%v", f.calls)
	}
}
