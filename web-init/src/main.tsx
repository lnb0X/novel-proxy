import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";

type Step = "token" | "account" | "storage" | "database" | "confirm" | "done";

interface Draft {
  username: string;
  password: string;
  adminPath: string;
  storageOn: boolean;
  driver: string;
  dsn: string;
}

async function post(path: string, body?: unknown): Promise<any> {
  const res = await fetch("/init/api" + path, {
    method: "POST",
    credentials: "same-origin",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (data.code !== 0) {
    throw new Error(data.msg ?? "请求失败");
  }
  return data.data;
}

function Wizard() {
  const [step, setStep] = useState<Step>("token");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [token, setToken] = useState("");
  const [draft, setDraft] = useState<Draft>({
    username: "",
    password: "",
    adminPath: "admin",
    storageOn: true,
    driver: "sqlite",
    dsn: "./data/cache.db",
  });
  const [adminPath, setAdminPath] = useState("");

  useEffect(() => {
    fetch("/init/api/state")
      .then((r) => r.json())
      .then((d) => {
        if (d.data?.completed) {
          setStep("done");
        }
      })
      .catch(() => undefined);
  }, []);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (step === "done") {
    return (
      <div className="card">
        <h1>已完成初始化</h1>
        <p>管理端路径：<code>/{adminPath || "（刷新查看）"}/</code></p>
        <p>管理端路由需重启服务后挂载，请重启 novel-proxy。</p>
      </div>
    );
  }

  return (
    <div className="card">
      <h1>novel-proxy 初始化</h1>
      {error ? <p className="error">{error}</p> : null}

      {step === "token" ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await post("/token", { token });
              setStep("account");
            });
          }}
        >
          <p>输入启动日志或 data/init-token.txt 中的初始化令牌：</p>
          <input value={token} onChange={(e) => setToken(e.target.value)} />
          <button disabled={busy}>下一步</button>
        </form>
      ) : null}

      {step === "account" ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await post("/account", {
                username: draft.username,
                password: draft.password,
                admin_path: draft.adminPath,
              });
              setStep("storage");
            });
          }}
        >
          <label>
            管理员用户名
            <input
              value={draft.username}
              onChange={(e) => setDraft({ ...draft, username: e.target.value })}
            />
          </label>
          <label>
            密码（至少 12 位，三类字符）
            <input
              type="password"
              value={draft.password}
              onChange={(e) => setDraft({ ...draft, password: e.target.value })}
            />
          </label>
          <label>
            管理端路径段
            <input
              value={draft.adminPath}
              onChange={(e) => setDraft({ ...draft, adminPath: e.target.value })}
            />
          </label>
          <button disabled={busy}>下一步</button>
        </form>
      ) : null}

      {step === "storage" ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await post("/storage", { enabled: draft.storageOn });
              if (!draft.storageOn) {
                setStep("confirm");
                return;
              }
              await post("/database", { driver: draft.driver, dsn: draft.dsn });
              setStep("confirm");
            });
          }}
        >
          <label>
            <input
              type="checkbox"
              checked={draft.storageOn}
              onChange={(e) => setDraft({ ...draft, storageOn: e.target.checked })}
            />
            启用文章缓存
          </label>
          {draft.storageOn ? (
            <>
              <label>
                驱动
                <select
                  value={draft.driver}
                  onChange={(e) => setDraft({ ...draft, driver: e.target.value })}
                >
                  <option value="sqlite">sqlite</option>
                  <option value="postgres">postgres</option>
                  <option value="mysql">mysql</option>
                  <option value="mongo">mongo</option>
                </select>
              </label>
              <label>
                DSN
                <input
                  value={draft.dsn}
                  onChange={(e) => setDraft({ ...draft, dsn: e.target.value })}
                />
              </label>
            </>
          ) : null}
          <button disabled={busy}>下一步</button>
        </form>
      ) : null}

      {step === "confirm" ? (
        <div>
          <p>确认后配置写入 data/instance.yaml，服务需重启以挂载管理端。</p>
          <button
            disabled={busy}
            onClick={() =>
              run(async () => {
                const d = await post("/confirm");
                setAdminPath(d.admin_path);
                setStep("done");
              })
            }
          >
            确认初始化
          </button>
        </div>
      ) : null}
    </div>
  );
}

createRoot(document.getElementById("root")!).render(<Wizard />);
