import { useCallback, useEffect, useState } from "react";
import {
  CheckCircle2,
  ChevronDown,
  CircleHelp,
  Clock3,
  Cloud,
  Layers3,
  ListFilter,
  LogOut,
  Moon,
  ShieldCheck,
  Settings2,
  Sun,
  TriangleAlert,
  X,
  XCircle,
} from "lucide-react";
import { Account, api, ApiError, State, Target } from "./api";
import { Brand, Spin } from "./components/ui";

import Login from "./pages/Login";
import Rules from "./pages/Rules";
import Accounts from "./pages/Accounts";
import Schedule from "./pages/Schedule";
import Logs from "./pages/Logs";
import System from "./pages/System";
import AccountModal from "./components/AccountModal";
import TargetModal from "./components/TargetModal";
import DeleteModal from "./components/DeleteModal";

type Tab = "rules" | "schedule" | "accounts" | "logs" | "system";
type Toast = { text: string; tone: "success" | "error" | "info" };
type Run = (key: string, action: () => Promise<void>) => Promise<void>;
const tabs: { id: Tab; title: string; icon: typeof Layers3 }[] = [
  { id: "rules", title: "放行规则", icon: Layers3 },
  { id: "schedule", title: "计划任务", icon: Clock3 },
  { id: "accounts", title: "云账号", icon: Cloud },
  { id: "logs", title: "操作日志", icon: ListFilter },
  { id: "system", title: "系统设置", icon: Settings2 },
];
export default function App() {
  const [state, setState] = useState<State | null>();
  const [connectionError, setConnectionError] = useState("");
  const [tab, setTab] = useState<Tab>(
    () =>
      (tabs.some((t) => t.id === location.hash.slice(1))
        ? location.hash.slice(1)
        : "rules") as Tab,
  );
  const [dark, setDark] = useState(
    () => localStorage.getItem("gatesync-theme") === "dark",
  );
  const [toast, setToast] = useState<Toast | null>(null);
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [accountModal, setAccountModal] = useState<Account | "new" | null>(
    null,
  );
  const [targetModal, setTargetModal] = useState<Target | "new" | null>(null);
  const [deleting, setDeleting] = useState<{
    type: "account" | "target";
    item: Account | Target;
  } | null>(null);
  const [userMenu, setUserMenu] = useState(false);
  const closeAccount = useCallback(() => setAccountModal(null), []);
  const closeTarget = useCallback(() => setTargetModal(null), []);
  const closeDelete = useCallback(() => setDeleting(null), []);
  const notify = (text: string, tone: Toast["tone"] = "success") =>
    setToast({ text, tone });
  const reload = useCallback(async () => {
    try {
      const s = await api<State>("/state");
      setState(s);
      setConnectionError("");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        setState(null);
      } else {
        setConnectionError(e instanceof Error ? e.message : "连接失败");
        throw e;
      }
    }
  }, []);
  useEffect(() => {
    void reload().catch(() => {});
  }, [reload]);
  useEffect(() => {
    const onHashChange = () => {
      const value = location.hash.slice(1);
      setTab(tabs.some((t) => t.id === value) ? (value as Tab) : "rules");
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);
  useEffect(() => {
    if (!state) return;
    const timer = setInterval(() => {
      void reload().catch(() => {});
    }, 30_000);
    return () => clearInterval(timer);
  }, [!!state, reload]);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    localStorage.setItem("gatesync-theme", dark ? "dark" : "light");
  }, [dark]);
  useEffect(() => {
    if (!toast) return;
    const timer = setTimeout(() => setToast(null), 6000);
    return () => clearTimeout(timer);
  }, [toast]);
  const run: Run = async (key, action) => {
    setBusy((v) => new Set(v).add(key));
    try {
      await action();
    } catch (e) {
      notify(e instanceof Error ? e.message : "操作失败", "error");
      if (e instanceof ApiError && e.status === 401) setState(null);
    } finally {
      setBusy((v) => {
        const next = new Set(v);
        next.delete(key);
        return next;
      });
    }
  };
  const nav = (id: Tab) => {
    setTab(id);
    location.hash = id;
  };
  const syncing = [...busy].some((v) => v.startsWith("sync"));
  const toastUI = toast && (
    <div
      className={`toast ${toast.tone}`}
      role={toast.tone === "error" ? "alert" : "status"}
    >
      {toast.tone === "success" ? (
        <CheckCircle2 size={19} />
      ) : toast.tone === "error" ? (
        <XCircle size={19} />
      ) : (
        <CircleHelp size={19} />
      )}
      <span>{toast.text}</span>
      <button
        className="icon-btn"
        onClick={() => setToast(null)}
        aria-label="关闭提示"
      >
        <X size={16} />
      </button>
    </div>
  );
  if (state === undefined)
    return (
      <div className="loading-page">
        <Brand />
        {connectionError ? (
          <>
            <p className="error-text">{connectionError}</p>
            <button
              className="btn"
              onClick={() => void reload().catch(() => {})}
            >
              重新连接
            </button>
          </>
        ) : (
          <Spin />
        )}
      </div>
    );
  if (state === null)
    return (
      <>
        <Login onLogin={reload} />
        {toastUI}
      </>
    );
  return (
    <>
      <header className="topbar">
        <div className="topbar-inner">
          <Brand />
          <span className="nav-divider" />
          <nav aria-label="主导航">
            {tabs.map((t) => (
              <button
                key={t.id}
                className={`nav-item ${tab === t.id ? "active" : ""}`}
                onClick={() => nav(t.id)}
              >
                <t.icon size={16} />
                {t.title}
              </button>
            ))}
          </nav>
          <div className="header-actions">
            <span className="local-label">
              <span className="status-dot" />
              本地部署
            </span>
            <button
              className="icon-btn theme-btn"
              onClick={() => setDark(!dark)}
              aria-label={dark ? "切换浅色主题" : "切换深色主题"}
            >
              {dark ? <Sun size={18} /> : <Moon size={18} />}
            </button>
            <div className="user-wrap">
              <button
                className="user-btn"
                onClick={() => setUserMenu(!userMenu)}
                aria-expanded={userMenu}
              >
                <span className="avatar">
                  {state.identity.charAt(0).toUpperCase()}
                </span>
                <span className="identity">{state.identity}</span>
                <ChevronDown size={13} />
              </button>
              {userMenu && (
                <div className="user-menu">
                  <p>{state.identity}</p>
                  <button
                    onClick={() =>
                      void run("logout", async () => {
                        await api("/auth/logout", "POST");
                        setState(null);
                        setUserMenu(false);
                      })
                    }
                  >
                    <LogOut size={15} />
                    退出登录
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </header>
      <main className="main">
        {connectionError && (
          <div className="notice danger">
            <TriangleAlert size={17} />
            与服务的连接暂时中断：{connectionError}
            <button
              className="text-btn"
              onClick={() => void reload().catch(() => {})}
            >
              重试
            </button>
          </div>
        )}
        {tab === "rules" && (
          <Rules
            state={state}
            syncing={syncing}
            busy={busy}
            onAdd={() =>
              state.accounts.length
                ? setTargetModal("new")
                : (nav("accounts"), setAccountModal("new"))
            }
            onEdit={setTargetModal}
            onDelete={(t) => setDeleting({ type: "target", item: t })}
            onDetect={() =>
              void run("detect", async () => {
                await api("/ip/detect", "POST");
                await reload();
                notify("公网 IP 检测完成");
              })
            }
            onSync={(id) =>
              void run(`sync-${id}`, async () => {
                await api(`/targets/${id}/sync`, "POST");
                await reload();
                notify("规则同步完成");
              })
            }
            onSyncAll={() =>
              void run("sync-all", async () => {
                const result = await api<{ success: number; failed: number }>(
                  "/sync",
                  "POST",
                );
                await reload();
                notify(
                  `${result.success} 个目标同步成功${result.failed ? `，${result.failed} 个失败，请查看操作日志` : ""}`,
                  result.failed ? "info" : "success",
                );
              })
            }
            onReorder={async (ids) => {
              await run("reorder", async () => {
                try {
                  await api("/targets/order", "PUT", { ids });
                } finally {
                  await reload();
                }
              });
            }}
            onToggle={(t) =>
              void run(`toggle-${t.id}`, async () => {
                await api(`/targets/${t.id}/auto`, "PATCH", {
                  enabled: !t.autoSync,
                });
                await reload();
              })
            }
          />
        )}
        {tab === "accounts" && (
          <Accounts
            state={state}
            busy={busy}
            onAdd={() => setAccountModal("new")}
            onEdit={setAccountModal}
            onDelete={(item) => setDeleting({ type: "account", item })}
            onTest={(item) =>
              void run(`test-${item.id}`, async () => {
                try {
                  const result = await api<{ message: string }>(
                    `/accounts/${item.id}/test`,
                    "POST",
                  );
                  notify(result.message);
                } finally {
                  await reload();
                }
              })
            }
          />
        )}
        {tab === "schedule" && (
          <Schedule
            state={state}
            busy={busy.has("settings")}
            onSave={(settings) =>
              void run("settings", async () => {
                await api("/settings", "PUT", settings);
                await reload();
                notify("计划任务设置已保存");
              })
            }
          />
        )}
        {tab === "logs" && <Logs />}
        {tab === "system" && (
          <System
            onSaved={async () => {
              await reload();
              notify("系统设置已保存，请重新登录");
            }}
          />
        )}
        <footer className="footer">
          <span>
            <ShieldCheck size={14} />
            仅维护 GateSync 创建的规则
          </span>
          <span>
            GateSync <span className="footer-dot">·</span> 腾讯云{" "}
            <span className="version">v0.1.1</span>
          </span>
        </footer>
      </main>
      {accountModal && (
        <AccountModal
          account={accountModal === "new" ? undefined : accountModal}
          onClose={closeAccount}
          onSaved={async () => {
            await reload();
            closeAccount();
            notify("云账号已保存");
          }}
        />
      )}
      {targetModal && (
        <TargetModal
          target={targetModal === "new" ? undefined : targetModal}
          state={state}
          onClose={closeTarget}
          onSaved={async (result) => {
            await reload();
            closeTarget();
            notify(
              result.status === "failed"
                ? `目标已保存，同步未完成：${result.error}`
                : "目标已保存并同步",
              result.status === "failed" ? "info" : "success",
            );
          }}
        />
      )}
      {deleting && (
        <DeleteModal
          item={deleting}
          onClose={closeDelete}
          onDeleted={async () => {
            await reload();
            closeDelete();
            notify("已删除");
          }}
        />
      )}
      {toastUI}
    </>
  );
}
