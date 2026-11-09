package app

import "net/http"

func (a *App) reorderTargets(w http.ResponseWriter, r *http.Request) {
	var input struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &input) {
		return
	}
	a.mutate.Lock()
	defer a.mutate.Unlock()
	targets, e := a.store.targets()
	if e != nil {
		fail(w, 500, "读取目标失败")
		return
	}
	// 完整校验成员，避免过期页面遗漏新目标或覆盖其他目标的顺序。
	remaining := make(map[string]bool, len(targets))
	for _, t := range targets {
		remaining[t.ID] = true
	}
	if len(input.IDs) != len(targets) {
		fail(w, 409, "目标列表已变化，请刷新后重新排序")
		return
	}
	for _, id := range input.IDs {
		if !remaining[id] {
			fail(w, 409, "目标列表已变化或包含重复项，请刷新后重新排序")
			return
		}
		delete(remaining, id)
	}
	if e := a.store.setMeta("target-order", input.IDs); e != nil {
		fail(w, 500, "保存排序失败")
		return
	}
	jsonResponse(w, 200, map[string]bool{"ok": true})
}
