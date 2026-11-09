import {
  CircleHelp,
  Cloud,
  LockKeyhole,
  Pencil,
  Plus,
  Trash2,
  Zap,
} from "lucide-react";
import { Account, date, State } from "../api";
import { Spin, Badge, Empty } from "../components/ui";

export default function Accounts({
  state,
  busy,
  onAdd,
  onEdit,
  onDelete,
  onTest,
}: {
  state: State;
  busy: Set<string>;
  onAdd: () => void;
  onEdit: (a: Account) => void;
  onDelete: (a: Account) => void;
  onTest: (a: Account) => void;
}) {
  return (
    <>
      <div className="page-title">
        <div>
          <div className="eyebrow muted">连接与凭证</div>
          <h1>
            云账号<span className="count-pill">{state.accounts.length}</span>
          </h1>
          <p>连接云厂商账号，为云资源的端口放行与规则同步提供授权。</p>
        </div>
        <button className="btn primary" onClick={onAdd}>
          <Plus size={16} />
          添加云账号
        </button>
      </div>
      <div className="notice info">
        <LockKeyhole size={17} />
        凭证加密保存，仅用于查询云资源和同步放行规则。密钥保存后不再回显。
      </div>
      <section className="card data-card">
        {state.accounts.length ? (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>账号名称</th>
                  <th>云厂商</th>
                  <th>SecretId</th>
                  <th>默认地域</th>
                  <th>放行目标</th>
                  <th>连接状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {state.accounts.map((a) => (
                  <tr key={a.id}>
                    <td>
                      <strong>{a.name}</strong>
                    </td>
                    <td>
                      <div className="inline">
                        <span className="provider-logo small">T</span>腾讯云
                      </div>
                    </td>
                    <td className="mono">{a.maskedKey}</td>
                    <td className="mono">{a.region}</td>
                    <td>{a.targetCount} 个</td>
                    <td>
                      <Badge status={a.status} />
                      {a.testedAt && <small>{date(a.testedAt)}</small>}
                    </td>
                    <td>
                      <div className="row-actions">
                        <button
                          className="text-btn"
                          onClick={() => onTest(a)}
                          disabled={busy.has(`test-${a.id}`)}
                        >
                          {busy.has(`test-${a.id}`) ? (
                            <Spin />
                          ) : (
                            <Zap size={13} />
                          )}
                          测试
                        </button>
                        <button
                          className="icon-btn"
                          aria-label={`编辑 ${a.name}`}
                          onClick={() => onEdit(a)}
                        >
                          <Pencil size={15} />
                        </button>
                        <button
                          className="icon-btn danger-icon"
                          aria-label={`删除 ${a.name}`}
                          onClick={() => onDelete(a)}
                        >
                          <Trash2 size={15} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            icon={Cloud}
            title="连接你的第一个云账号"
            text="选择云厂商并填写访问凭证，授予所需的资源查询和规则管理权限。"
            action={
              <button className="btn" onClick={onAdd}>
                <Plus size={15} />
                添加云账号
              </button>
            }
          />
        )}
        <div className="table-footer">
          <span>
            <LockKeyhole size={13} />
            AES-256-GCM 加密
          </span>
          <span>{state.accounts.length} 个账号</span>
        </div>
      </section>
      <div className="policy-note">
        <CircleHelp size={17} />
        <p>
          连接测试只验证读取权限，写入权限将在实际同步时验证。
          <br />
          <span>
            删除账号时默认保留云端规则，也可选择撤销本工具创建的规则。
          </span>
        </p>
      </div>
    </>
  );
}
