package app

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestLogGroupingPreservesBoundariesAndDetails(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	if _, e := a.store.db.Exec("DELETE FROM logs"); e != nil {
		t.Fatal(e)
	}
	add := func(l LogEntry, n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			if e := a.store.addLog(l); e != nil {
				t.Fatal(e)
			}
		}
	}
	unchanged := LogEntry{Kind: "ip", Trigger: "auto", Object: "公网 IP", Detail: "公网 IP 无变化：8.8.8.8", Success: true}
	add(unchanged, 25)
	add(LogEntry{Kind: "ip", Trigger: "auto", Object: "公网 IP", Detail: "检测失败", Success: false}, 1)
	add(unchanged, 2)
	add(LogEntry{Kind: "sync", Trigger: "auto", Object: "目标", Detail: "同步成功", Success: true}, 1)
	add(unchanged, 3)
	manual := unchanged
	manual.Trigger = "manual"
	add(manual, 2)
	other := unchanged
	other.Detail = "公网 IP 无变化：9.9.9.9"
	add(other, 2)
	other.Changes = []string{"+ 不应折叠包含变更的记录"}
	add(other, 2)
	type response struct {
		Items    []logGroup `json:"items"`
		Total    int        `json:"total"`
		RawTotal int        `json:"rawTotal"`
	}
	get := func(path string) response {
		t.Helper()
		w := request(t, a, "GET", path, nil, cookie)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out response
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	grouped := get("/api/logs?group=true")
	want := []int{1, 1, 2, 2, 3, 1, 2, 1, 25}
	if grouped.Total != 9 || grouped.RawTotal != 38 || len(grouped.Items) != len(want) {
		t.Fatalf("分组或原始计数错误：%+v", grouped)
	}
	for i, count := range want {
		if grouped.Items[i].Count != count || grouped.Items[i].FirstTime == "" {
			t.Fatalf("第 %d 组错误：%+v", i, grouped.Items[i])
		}
	}
	filtered := get("/api/logs?group=true&kind=ip")
	if filtered.Total != 8 || filtered.RawTotal != 37 || filtered.Items[4].Count != 3 || filtered.Items[5].Count != 2 {
		t.Fatal("类型筛选跨过同步记录合并了非连续段", filtered)
	}
	if both := get("/api/logs?group=true&kind=ip&kind=sync"); both.Total != 9 || both.RawTotal != 38 {
		t.Fatal("执行记录类型筛选错误", both)
	}
	if failed := get("/api/logs?group=true&result=failed"); failed.Total != 1 || failed.Items[0].Success {
		t.Fatal("失败记录被折叠或隐藏", failed)
	}
	if searched := get("/api/logs?group=true&q=8.8.8.8&trigger=auto"); searched.Total != 3 || searched.RawTotal != 30 {
		t.Fatal("搜索或触发筛选错误", searched)
	}
	group := grouped.Items[8]
	rangePath := fmt.Sprintf("/api/logs?firstId=%d&lastId=%d", group.FirstID, group.ID)
	first, second := get(rangePath), get(rangePath+"&page=2")
	if first.Total != 25 || len(first.Items) != 20 || len(second.Items) != 5 || second.Items[4].ID != group.FirstID || first.Items[0].ID != group.ID {
		t.Fatal("展开明细遗漏了跨页记录", first, second)
	}
	if raw := get("/api/logs"); raw.Total != 38 || len(raw.Items) != 20 {
		t.Fatal("原始列表接口不兼容", raw)
	}
	w := request(t, a, "GET", "/api/logs/export?group=true", nil, cookie)
	records, e := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\ufeff"))).ReadAll()
	if e != nil || w.Code != 200 || len(records) != 39 {
		t.Fatal("CSV 未保留全部原始记录", e, len(records))
	}
	for _, path := range []string{"firstId=0&lastId=9", "firstId=9&lastId=1", "firstId=x&lastId=9", "firstId=1"} {
		if w := request(t, a, "GET", "/api/logs?"+path, nil, cookie); w.Code != 400 {
			t.Fatal("接受了无效明细范围", path, w.Code)
		}
	}
	if w := request(t, a, "GET", rangePath, nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatal("明细绕过了登录校验", w.Code)
	}
}

func TestLogGroupingPaginatesAfterAggregation(t *testing.T) {
	a, _ := testApp(t)
	cookie := loginCookie(t, a)
	if _, e := a.store.db.Exec("DELETE FROM logs"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 65; i++ {
		l := LogEntry{Kind: "sync", Trigger: "auto", Success: true, Detail: "同步成功"}
		if i >= 25 {
			l.Kind, l.Detail = "ip", "公网 IP 无变化：8.8.8.8"
		}
		if e := a.store.addLog(l); e != nil {
			t.Fatal(e)
		}
	}
	for page, length := range map[int]int{1: 20, 2: 6, 3: 0} {
		w := request(t, a, "GET", fmt.Sprintf("/api/logs?group=true&page=%d", page), nil, cookie)
		var out struct {
			Items    []logGroup `json:"items"`
			Total    int        `json:"total"`
			RawTotal int        `json:"rawTotal"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil || w.Code != 200 || out.Total != 26 || out.RawTotal != 65 || len(out.Items) != length {
			t.Fatal("聚合分页或越界总数错误", page, w.Code, w.Body.String())
		}
		if page == 1 && out.Items[0].Count != 40 {
			t.Fatal("连续段在原始分页处被切断")
		}
	}
}
