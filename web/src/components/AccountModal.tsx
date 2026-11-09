import { useState } from "react";
import type { FormEvent } from "react";
import {
  CheckCircle2,
  LockKeyhole,
  ShieldCheck,
  TriangleAlert,
  Zap,
} from "lucide-react";
import { Account, api, Region } from "../api";
import { Spin, Modal, RegionField, Select } from "./ui";

export default function AccountModal({
  account,
  onClose,
  onSaved,
}: {
  account?: Account;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [form, setForm] = useState({
    resourceType: account?.resourceType || "security-group",
    name: account?.name || "",
    region: account?.region || "ap-guangzhou",
    secretId: "",
    secretKey: "",
  });
  const [regionOptions, setRegionOptions] = useState<Region[]>([]);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [test, setTest] = useState("");
  const update = (k: string, v: string) => {
    setForm((f) => ({ ...f, [k]: v }));
    setTest("");
    setError("");
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy("save");
    setError("");
    try {
      await api(
        account ? `/accounts/${account.id}` : "/accounts",
        account ? "PUT" : "POST",
        form,
      );
      await onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setBusy("");
    }
  };
  const testConnection = async () => {
    setBusy("test");
    setError("");
    setTest("");
    try {
      const v = await api<{ count: number; message: string }>(
        "/accounts/test",
        "POST",
        { ...form, id: account?.id || "" },
      );
      setTest(v.message);
    } catch (e) {
      setError(e instanceof Error ? e.message : "连接失败");
    } finally {
      setBusy("");
    }
  };
  const loadRegions = async () => {
    setBusy("regions");
    setError("");
    try {
      setRegionOptions(
        await api<Region[]>("/accounts/regions", "POST", {
          ...form,
          id: account?.id || "",
        }),
      );
    } catch (e) {
      setError(
        (e instanceof Error ? e.message : "读取地域失败") +
          "，可手动输入地域。",
      );
    } finally {
      setBusy("");
    }
  };
  return (
    <Modal
      title={account ? "编辑云账号" : "添加云账号"}
      subtitle="连接云厂商账号，安全地管理你的授权凭证"
      onClose={onClose}
      busy={!!busy}
    >
      <form onSubmit={submit}>
        <fieldset disabled={!!busy} className="modal-body">
          <label className="field">
            云厂商
            <div className="provider-field">
              <span className="provider-logo">T</span>
              <strong>腾讯云</strong>
              <span className="badge good">已支持</span>
            </div>
          </label>
          <label className="field">
            默认目标类型
            <Select
              label="默认目标类型"
              value={form.resourceType}
              onChange={(value) => {
                update("resourceType", value);
                setRegionOptions([]);
              }}
              options={[
                { value: "security-group", label: "VPC 安全组" },
                { value: "lighthouse", label: "Lighthouse 防火墙" },
              ]}
            />
            <span className="field-hint">
              用于连接测试；添加目标时仍可选择另一类型。
            </span>
          </label>
          <label className="field">
            账号名称
            <input
              value={form.name}
              onChange={(e) => update("name", e.target.value)}
              placeholder="例如：办公室云账号"
              required
              maxLength={60}
            />
          </label>
          <label className="field">
            SecretId
            <input
              className="mono"
              value={form.secretId}
              onChange={(e) => update("secretId", e.target.value)}
              placeholder={
                account ? `${account.maskedKey}（留空不修改）` : "AKID…"
              }
              required={!account}
              maxLength={256}
              autoComplete="off"
            />
          </label>
          <label className="field">
            SecretKey
            <input
              type="password"
              value={form.secretKey}
              onChange={(e) => update("secretKey", e.target.value)}
              placeholder={account ? "留空表示不修改" : "请输入 SecretKey"}
              required={!account}
              maxLength={256}
              autoComplete="new-password"
            />
            <span className="field-hint">
              <LockKeyhole size={12} />
              加密保存，保存后不可再查看
            </span>
          </label>
          <RegionField
            value={form.region}
            options={regionOptions}
            onChange={(v) => update("region", v)}
          />
          <button
            type="button"
            className="text-btn"
            onClick={() => void loadRegions()}
            disabled={!!busy}
          >
            {busy === "regions" ? <Spin /> : null}读取可用地域
          </button>
          <div className="notice info small-notice">
            <ShieldCheck size={17} />
            建议使用仅授予所需资源读取和规则新增、删除权限的子账号。
          </div>
          {error && (
            <div className="notice danger" role="alert">
              <TriangleAlert size={16} />
              {error}
            </div>
          )}
          {test && (
            <div className="notice success">
              <CheckCircle2 size={16} />
              {test}
            </div>
          )}
          <button
            type="button"
            className="btn"
            disabled={!!busy}
            onClick={() => void testConnection()}
          >
            {busy === "test" ? <Spin /> : <Zap size={15} />}测试连接
          </button>
        </fieldset>
        <div className="modal-footer">
          <button
            type="button"
            className="btn"
            onClick={onClose}
            disabled={!!busy}
          >
            取消
          </button>
          <button className="btn primary" disabled={!!busy}>
            {busy === "save" ? <Spin /> : null}保存账号
          </button>
        </div>
      </form>
    </Modal>
  );
}
