import { useState } from "react";
import { Trash2 } from "lucide-react";
import { Account, api, Target } from "../api";
import { Spin, Modal } from "./ui";

export default function DeleteModal({
  item,
  onClose,
  onDeleted,
}: {
  item: { type: "account" | "target"; item: Account | Target };
  onClose: () => void;
  onDeleted: () => Promise<void>;
}) {
  const [revoke, setRevoke] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const account = item.type === "account";
  const name = account
    ? (item.item as Account).name
    : (item.item as Target).groupName || (item.item as Target).groupId;
  const remove = async () => {
    setBusy(true);
    setError("");
    try {
      await api(
        `/${account ? "accounts" : "targets"}/${item.item.id}?revoke=${revoke}`,
        "DELETE",
      );
      await onDeleted();
    } catch (e) {
      setError(e instanceof Error ? e.message : "删除失败");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      title={account ? "删除云账号" : "删除放行目标"}
      onClose={onClose}
      busy={busy}
    >
      <div className="modal-body">
        <div className="delete-warning">
          <span className="delete-icon">
            <Trash2 size={21} />
          </span>
          <p>
            将删除「<strong>{name}</strong>」
            {account
              ? `及其 ${(item.item as Account).targetCount} 个放行目标`
              : ""}
            。<br />
            <span className="muted">云端资源和已有规则默认保留。</span>
          </p>
        </div>
        <label className="check-label revoke-label">
          <input
            type="checkbox"
            checked={revoke}
            disabled={busy}
            onChange={(e) => setRevoke(e.target.checked)}
          />
          同时撤销云端由 GateSync 添加的规则
        </label>
        <p className="field-hint">
          {revoke
            ? "撤销失败时会保留本地记录。若部分规则已撤销，可重试完成剩余操作。"
            : "不勾选时云端规则保持不变，可在云控制台手动撤销。"}
        </p>
        {error && (
          <div className="notice danger" role="alert">
            {error}
          </div>
        )}
      </div>
      <div className="modal-footer">
        <button className="btn" disabled={busy} onClick={onClose}>
          取消
        </button>
        <button
          className="btn danger"
          disabled={busy}
          onClick={() => void remove()}
        >
          {busy ? <Spin /> : null}确认删除
        </button>
      </div>
    </Modal>
  );
}
