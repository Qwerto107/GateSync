import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  ArrowUpRight,
  Globe2,
  Plus,
  RefreshCw,
  TriangleAlert,
  X,
} from "lucide-react";
import { api, Port, State, Target, Region, FixedIP } from "../api";
import { Spin, Modal, RegionField, Select } from "./ui";

export default function TargetModal({
  target,
  state,
  onClose,
  onSaved,
}: {
  target?: Target;
  state: State;
  onClose: () => void;
  onSaved: (t: Target) => Promise<void>;
}) {
  const first = state.accounts[0];
  const [form, setForm] = useState({
    resourceType:
      target?.resourceType || first?.resourceType || "security-group",
    name: target?.name || "",
    accountId: target?.accountId || first?.id || "",
    region: target?.region || first?.region || "ap-guangzhou",
    groupId: target?.groupId || "",
    groupName: target?.groupName || "",
    ports: target?.ports || ([] as Port[]),
    ipMode: target?.ipMode || "dynamic",
    fixedIps:
      target?.fixedIps ??
      (target?.ipMode === "fixed" && target.fixedCidr
        ? [{ cidr: target.fixedCidr, name: "" }]
        : ([] as FixedIP[])),
    autoSync: target?.autoSync ?? true,
    note: target?.note || "",
  });
  const [customEnabled, setCustomEnabled] = useState(
    !!target?.fixedIps?.length || target?.ipMode === "fixed",
  );
  const updateIP = (index: number, field: keyof FixedIP, value: string) => {
    setForm((f) => ({
      ...f,
      fixedIps: f.fixedIps.map((ip, i) =>
        i === index ? { ...ip, [field]: value } : ip,
      ),
    }));
  };
  const [regionOptions, setRegionOptions] = useState<Region[]>([]);
  const [regionError, setRegionError] = useState("");
  const isLighthouse = form.resourceType === "lighthouse";
  const resourceLabel = isLighthouse ? "Lighthouse 实例" : "安全组";
  const [groups, setGroups] = useState<{ id: string; name: string }[]>([]);
  const [groupsError, setGroupsError] = useState("");
  const [loadingGroups, setLoadingGroups] = useState(false);
  const [manual, setManual] = useState(!!target);
  const [protocol, setProtocol] = useState("TCP");
  const [port, setPort] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!form.accountId || target) return;
    let cancelled = false;
    setRegionOptions([]);
    setRegionError("");
    void api<Region[]>(
      `/accounts/${form.accountId}/regions?resourceType=${form.resourceType}`,
    )
      .then((v) => {
        if (!cancelled) setRegionOptions(v);
      })
      .catch((e) => {
        if (!cancelled) setRegionError(e.message + "，可手动输入地域。");
      });
    return () => {
      cancelled = true;
    };
  }, [form.accountId, form.resourceType, target]);
  useEffect(() => {
    if (!form.accountId || !form.region || manual) return;
    let cancel = false;
    setLoadingGroups(true);
    setGroupsError("");
    setGroups([]);
    const timer = setTimeout(() => {
      void api<{ id: string; name: string }[]>(
        `/accounts/${form.accountId}/groups?region=${encodeURIComponent(form.region)}&resourceType=${form.resourceType}`,
      )
        .then((v) => {
          if (!cancel) setGroups(v);
        })
        .catch((e) => {
          if (!cancel) setGroupsError(e.message);
        })
        .finally(() => {
          if (!cancel) setLoadingGroups(false);
        });
    }, 300);
    return () => {
      cancel = true;
      clearTimeout(timer);
    };
  }, [form.accountId, form.region, manual, form.resourceType]);
  const addPort = (): boolean => {
    const values = port
      .trim()
      .split("-")
      .map((v) => (/^\d+$/.test(v) ? Number(v) : NaN));
    if (
      values.length > 2 ||
      values.some((v) => !Number.isInteger(v) || v < 1 || v > 65535) ||
      (values.length === 2 && values[0] > values[1])
    ) {
      setError("请输入 1–65535 的端口，或有效范围（如 8000-8100）");
      return false;
    }
    const value =
      values.length === 2 && values[0] !== values[1]
        ? values.join("-")
        : String(values[0]);
    if (form.ports.some((p) => p.protocol === protocol && p.value === value)) {
      setError("该端口已添加");
      return false;
    }
    setForm((f) => ({ ...f, ports: [...f.ports, { protocol, value }] }));
    setPort("");
    setError("");
    return true;
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (form.ipMode !== "dynamic" && !customEnabled) {
      setError("请至少选择一个 IP 来源");
      return;
    }
    if (customEnabled && !form.fixedIps.length) {
      setError("请添加自定义 IP");
      return;
    }
    if (port.trim()) {
      if (addPort()) setError("端口已加入，请再次点击保存并同步");
      return;
    }
    if (!form.ports.length) {
      setError("请至少添加一个放行端口");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const result = await api<Target>(
        target ? `/targets/${target.id}` : "/targets",
        target ? "PUT" : "POST",
        { ...form, fixedIps: customEnabled ? form.fixedIps : [] },
      );
      await onSaved(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      title={target ? "编辑放行目标" : "添加放行目标"}
      subtitle="选择安全组或 Lighthouse 实例，设置需要放行的端口"
      onClose={onClose}
      wide
      busy={busy}
    >
      <form onSubmit={submit}>
        <fieldset disabled={busy} className="modal-body">
          <label className="field">
            目标名称 <span className="optional">可选</span>
            <input
              value={form.name}
              maxLength={60}
              placeholder="例如：办公室 SSH"
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>
          <label className="field">
            目标类型
            <Select
              label="目标类型"
              value={form.resourceType}
              disabled={!!target}
              options={[
                { value: "security-group", label: "VPC 安全组" },
                { value: "lighthouse", label: "Lighthouse 防火墙" },
              ]}
              onChange={(value) =>
                setForm({
                  ...form,
                  resourceType: value,
                  groupId: "",
                  groupName: "",
                })
              }
            />
          </label>
          <div className="form-grid">
            <label className="field">
              云账号
              <Select
                label="云账号"
                value={form.accountId}
                disabled={!!target}
                required
                searchable
                options={state.accounts.map((a) => ({
                  value: a.id,
                  label: a.name,
                }))}
                onChange={(value) => {
                  const a = state.accounts.find((v) => v.id === value)!;
                  setForm({
                    ...form,
                    accountId: a.id,
                    resourceType: a.resourceType || "security-group",
                    region: a.region,
                    groupId: "",
                    groupName: "",
                  });
                }}
              />
            </label>
            <RegionField
              value={form.region}
              options={regionOptions}
              disabled={!!target}
              onChange={(v) =>
                setForm({ ...form, region: v, groupId: "", groupName: "" })
              }
            />
          </div>
          {regionError && (
            <p className="field-hint error-text">{regionError}</p>
          )}
          <label className="field">
            {resourceLabel}
            {manual ? (
              <input
                className="mono"
                value={form.groupId}
                placeholder={isLighthouse ? "lhins-xxxxxxxx" : "sg-xxxxxxxx"}
                disabled={!!target}
                required
                onChange={(e) =>
                  setForm({
                    ...form,
                    groupId: e.target.value.trim(),
                    groupName: "",
                  })
                }
              />
            ) : (
              <Select
                label={resourceLabel}
                value={form.groupId}
                required
                searchable
                placeholder={
                  loadingGroups
                    ? `正在读取${resourceLabel}…`
                    : groups.length
                      ? `请选择${resourceLabel}`
                      : `暂无${resourceLabel}`
                }
                options={groups.map((g) => ({
                  value: g.id,
                  label: `${g.name}（${g.id}）`,
                }))}
                onChange={(value) =>
                  setForm({
                    ...form,
                    groupId: value,
                    groupName: groups.find((g) => g.id === value)?.name || "",
                  })
                }
              />
            )}
            {!target && (
              <button
                type="button"
                className="text-btn field-link"
                onClick={() => setManual(!manual)}
              >
                {manual
                  ? `返回${resourceLabel}列表选择`
                  : `找不到？直接输入${resourceLabel} ID`}
                <ArrowUpRight size={12} />
              </button>
            )}
            {target && (
              <span className="field-hint">
                更换类型、账号、地域或云端资源时，请创建新目标。
              </span>
            )}
            {groupsError && !manual && (
              <span className="error-text field-hint">
                {groupsError}，可切换为手动输入。
              </span>
            )}
          </label>
          <div className="field">
            <label className="field-label" htmlFor="target-port">
              放行端口
            </label>
            <div className="port-editor">
              {form.ports.map((p, i) => (
                <span
                  className="editable-port mono"
                  key={`${p.protocol}-${p.value}`}
                >
                  {p.protocol} {p.value}
                  <button
                    type="button"
                    aria-label={`移除 ${p.protocol} ${p.value}`}
                    onClick={() =>
                      setForm({
                        ...form,
                        ports: form.ports.filter((_, n) => n !== i),
                      })
                    }
                  >
                    <X size={12} />
                  </button>
                </span>
              ))}
            </div>
            <div className="port-input">
              <Select
                label="端口协议"
                value={protocol}
                onChange={setProtocol}
                options={[
                  { value: "TCP", label: "TCP" },
                  { value: "UDP", label: "UDP" },
                ]}
              />
              <input
                id="target-port"
                value={port}
                placeholder="如 443 或 8000-8100"
                onChange={(e) => setPort(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addPort();
                  }
                }}
              />
              <button className="btn" type="button" onClick={addPort}>
                <Plus size={14} />
                添加
              </button>
            </div>
          </div>
          <div className="field">
            <span className="field-label">放行 IP</span>
            <span className="field-hint">
              可同时选择多个来源，所有 IP
              共用上方端口，重复地址自动去重。保存时使用缓存公网
              IP，不主动查询。
            </span>
            <div
              className={`radio-card ${form.ipMode === "dynamic" ? "chosen" : ""}`}
            >
              <label>
                <input
                  type="checkbox"
                  checked={form.ipMode === "dynamic"}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      ipMode: e.target.checked ? "dynamic" : "fixed",
                    })
                  }
                />
                <span>
                  跟随当前公网 IP
                  <small className="mono">
                    {state.ip.ip
                      ? `${state.ip.ip}/32`
                      : "尚无缓存，请先重新检测"}
                  </small>
                </span>
                <Globe2 size={18} />
              </label>
            </div>
            <div className={`radio-card ${customEnabled ? "chosen" : ""}`}>
              <label>
                <input
                  type="checkbox"
                  checked={customEnabled}
                  onChange={(e) => {
                    setCustomEnabled(e.target.checked);
                    if (e.target.checked && !form.fixedIps.length)
                      setForm({ ...form, fixedIps: [{ cidr: "", name: "" }] });
                  }}
                />
                <span>
                  自定义 IP<small>固定 IPv4 地址或 CIDR 网段，最多 20 项</small>
                </span>
              </label>
              {customEnabled && (
                <div className="custom-ip-list">
                  {form.fixedIps.map((ip, index) => (
                    <div className="custom-ip-row" key={index}>
                      <input
                        className="mono"
                        aria-label={`自定义 IP ${index + 1}`}
                        required
                        value={ip.cidr}
                        placeholder="1.2.3.4 或 1.2.3.0/24"
                        onChange={(e) =>
                          updateIP(index, "cidr", e.target.value)
                        }
                      />
                      <input
                        aria-label={`IP 名称 ${index + 1}`}
                        value={ip.name || ""}
                        placeholder="名称（可选）"
                        maxLength={30}
                        onChange={(e) =>
                          updateIP(index, "name", e.target.value)
                        }
                      />
                      <button
                        className="icon-btn danger-icon"
                        type="button"
                        aria-label={`移除 IP ${index + 1}`}
                        onClick={() =>
                          setForm({
                            ...form,
                            fixedIps: form.fixedIps.filter(
                              (_, i) => i !== index,
                            ),
                          })
                        }
                      >
                        <X size={15} />
                      </button>
                    </div>
                  ))}
                  <button
                    className="btn"
                    type="button"
                    disabled={form.fixedIps.length >= 20}
                    onClick={() =>
                      setForm({
                        ...form,
                        fixedIps: [...form.fixedIps, { cidr: "", name: "" }],
                      })
                    }
                  >
                    <Plus size={14} />
                    添加 IP
                  </button>
                </div>
              )}
            </div>
          </div>
          <label className="check-label">
            <input
              type="checkbox"
              checked={form.autoSync}
              onChange={(e) => setForm({ ...form, autoSync: e.target.checked })}
            />
            加入自动同步，按计划检查并更新
          </label>
          <label className="field">
            备注<span className="optional">可选</span>
            <input
              value={form.note}
              onChange={(e) => setForm({ ...form, note: e.target.value })}
              placeholder="例如：办公室 SSH"
              maxLength={100}
            />
            <span className="field-hint">
              云端规则显示此备注并附加简短归属标记；过长内容会自动截短。
            </span>
          </label>
          {error && (
            <div className="notice danger" role="alert">
              <TriangleAlert size={16} />
              {error}
            </div>
          )}
        </fieldset>
        <div className="modal-footer">
          <button
            className="btn"
            type="button"
            disabled={busy}
            onClick={onClose}
          >
            取消
          </button>
          <button className="btn primary" disabled={busy}>
            {busy ? <Spin /> : <RefreshCw size={15} />}
            {target ? "保存并同步" : "添加并同步"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
