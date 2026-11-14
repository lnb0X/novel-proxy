import { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./tokens.css";
import "./wizard.css";

// 四步向导：1 令牌、2 账号、3 缓存、4 确认。
// 密码规则与管理端后端 ValidatePassword 逐条对应，后端为最终裁判。
// 第 3 步按驱动收集结构化字段，前端拼 DSN；后端接口仍收 { driver, dsn }。
interface DbFields {
  host: string;
  port: string;
  user: string;
  password: string;
  showPassword: boolean;
  dbName: string;
  authSource: string;
}

interface Draft {
  username: string;
  password: string;
  showPassword: boolean;
  adminPath: string;
  storageOn: boolean;
  driver: string;
  path: string;
  db: DbFields;
}

const WEAK = ["password", "qwerty", "123456", "admin", "letmein"];
const RESERVED = [
  "init",
  "api",
  "assets",
  "static",
  "health",
  "login",
  "admin",
  "favicon.ico",
  "robots.txt",
];
const STEP_TITLES = ["令牌", "账号", "缓存", "确认"];

// 各驱动的默认端口，随驱动切换自动填入，可改。
const DEFAULT_PORT: Record<string, string> = {
  postgres: "5432",
  mysql: "3306",
  mongo: "27017",
};

const SQLITE_DEFAULT_PATH = "./data/cache.db";

const enc = encodeURIComponent;

// buildDSN 按驱动拼真实 DSN；用户名与密码等特殊字符必须编码。
function buildDSN(d: Draft): string {
  const f = d.db;
  switch (d.driver) {
    case "sqlite":
      return d.path;
    case "postgres":
      return (
        `postgres://${enc(f.user)}:${enc(f.password)}` +
        `@${f.host}:${f.port}/${enc(f.dbName)}?sslmode=disable&connect_timeout=5`
      );
    case "mysql":
      return (
        `${enc(f.user)}:${enc(f.password)}@tcp(${f.host}:${f.port})/` +
        `${enc(f.dbName)}?charset=utf8mb4&parseTime=true&loc=Local&timeout=5s`
      );
    case "mongo":
      return (
        `mongodb://${enc(f.user)}:${enc(f.password)}` +
        `@${f.host}:${f.port}/${enc(f.dbName)}?authSource=${enc(f.authSource)}` +
        `&connectTimeoutMS=5000`
      );
    default:
      return "";
  }
}

// buildDSNMasked 仅供展示：密码段打码成 ****。
function buildDSNMasked(d: Draft): string {
  const f = d.db;
  switch (d.driver) {
    case "sqlite":
      return d.path;
    case "postgres":
      return (
        `postgres://${enc(f.user)}:****` +
        `@${f.host}:${f.port}/${enc(f.dbName)}?sslmode=disable&connect_timeout=5`
      );
    case "mysql":
      return (
        `${enc(f.user)}:****@tcp(${f.host}:${f.port})/` +
        `${enc(f.dbName)}?charset=utf8mb4&parseTime=true&loc=Local&timeout=5s`
      );
    case "mongo":
      return (
        `mongodb://${enc(f.user)}:****` +
        `@${f.host}:${f.port}/${enc(f.dbName)}?authSource=${enc(f.authSource)}` +
        `&connectTimeoutMS=5000`
      );
    default:
      return "";
  }
}

// dbFieldsError 结构化字段校验：除 authSource 外全部必填，不满足时
// 返回具体原因。
function dbFieldsError(d: Draft): string {
  if (d.driver === "") {
    return "请选择驱动";
  }
  if (d.driver === "sqlite") {
    return d.path.trim() === "" ? "文件路径不能为空" : "";
  }
  const f = d.db;
  if (f.host.trim() === "") {
    return "主机不能为空";
  }
  const port = Number(f.port);
  if (!/^\d{1,5}$/.test(f.port.trim()) || port < 1 || port > 65535) {
    return "端口需为 1 到 65535 的数字";
  }
  if (f.user.trim() === "") {
    return "用户名不能为空";
  }
  if (f.password === "") {
    return "密码不能为空";
  }
  if (f.dbName.trim() === "") {
    return "数据库名不能为空";
  }
  return "";
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

// randomPath 生成 32 位小写字母数字的管理端路径段，用户可改。
function randomPath(): string {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  const buf = new Uint32Array(32);
  crypto.getRandomValues(buf);
  return Array.from(buf, (n) => chars[n % chars.length]).join("");
}

// 密码四条规则，判定逻辑与后端 ValidatePassword 一致：
// 长度按字节算；符号是所有非小写、非大写、非数字的字符。
function passwordRules(username: string, pw: string) {
  const bytes = new TextEncoder().encode(pw).length;
  const hasLower = /\p{Ll}/u.test(pw);
  const hasUpper = /\p{Lu}/u.test(pw);
  const hasDigit = /\p{Nd}/u.test(pw);
  const isCls = (ch: string) =>
    /\p{Ll}/u.test(ch) || /\p{Lu}/u.test(ch) || /\p{Nd}/u.test(ch);
  const hasSymbol = Array.from(pw).some((ch) => !isCls(ch));
  const classes = [hasLower, hasUpper, hasDigit, hasSymbol].filter(Boolean).length;
  const lu = username.toLowerCase();
  const lp = pw.toLowerCase();
  return [
    { ok: bytes >= 12, label: "长度至少 12 位" },
    { ok: classes >= 3, label: "包含小写、大写、数字、符号中的至少三类" },
    {
      ok: username === "" || (lp !== lu && !lp.includes(lu)),
      label: "不得与用户名相同或包含用户名",
    },
    {
      ok: !WEAK.some((w) => lp.includes(w)),
      label: "不包含常见弱口令（password、qwerty、123456、admin、letmein）",
    },
  ];
}

// pathReason 管理端路径校验，不满足时返回具体原因，满足返回空串。
// 服务端只收 1 到 32 位字母数字下划线连字符，这里收紧到
// 16-32 位小写字母数字；超过 32 位会被后端拒绝，必须当场拦下。
function pathReason(p: string): string {
  if (p.length === 0) {
    return "路径不能为空";
  }
  // 保留字先于长度：保留字都短于 16 位，先查长度会把它挡住。
  const hit = RESERVED.find((r) => p === r);
  if (hit) {
    return hit + " 是保留字，不能使用";
  }
  if (!/^[a-z0-9]+$/.test(p)) {
    return "只能使用小写字母和数字";
  }
  if (p.length < 16) {
    return "长度需至少 16 位";
  }
  if (p.length > 32) {
    return "长度不能超过 32 位（服务端限制）";
  }
  return "";
}

function Wizard() {
  const [step, setStep] = useState(1);
  const [done, setDone] = useState(false);
  const [completed, setCompleted] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [token, setToken] = useState("");
  const [draft, setDraft] = useState<Draft>({
    username: "",
    password: "",
    showPassword: false,
    adminPath: "",
    storageOn: false,
    driver: "",
    path: "",
    db: {
      host: "",
      port: "",
      user: "",
      password: "",
      showPassword: false,
      dbName: "",
      authSource: "admin",
    },
  });
  // 测试连接状态：记录通过时的 DSN，字段再动一下就失效。
  const [tested, setTested] = useState<{ dsn: string; ok: boolean; msg: string } | null>(null);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    setDraft((d) => ({ ...d, adminPath: randomPath() }));
    fetch("/init/api/state")
      .then((r) => r.json())
      .then((d) => {
        if (d.data?.completed) {
          setCompleted(true);
        }
      })
      .catch(() => undefined);
  }, []);

  const rules = useMemo(
    () => passwordRules(draft.username, draft.password),
    [draft.username, draft.password],
  );
  const rulesOK = rules.every((r) => r.ok);
  const pErr = pathReason(draft.adminPath);
  const adminURL = window.location.origin + "/" + draft.adminPath + "/";

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

  if (completed) {
    return (
      <div className="card">
        <h1>本服务已完成初始化</h1>
        <p>请访问你的管理端地址（路径段在 data/instance.yaml 的 server.admin_path）。</p>
      </div>
    );
  }

  if (done) {
    return (
      <div className="card">
        <h1>初始化完成</h1>
        <p>管理端地址：</p>
        <p>
          <a className="admin-link" href={"/" + draft.adminPath + "/"}>
            {adminURL}
          </a>
        </p>
        <p className="muted">配置已写入 data/instance.yaml，服务已热挂载生效，无需重启。</p>
      </div>
    );
  }

  // 步骤指示：1/4 到 4/4。
  const stepsHeader = (
    <div className="steps">
      <span className="steps-count">{step} / 4</span>
      <div className="steps-dots">
        {STEP_TITLES.map((t, i) => {
          const cls =
            i + 1 < step ? "step-dot past" : i + 1 === step ? "step-dot cur" : "step-dot";
          return (
            <span key={t} className={cls}>
              <span className="step-num">{i + 1}</span>
              <span className="step-name">{t}</span>
            </span>
          );
        })}
      </div>
    </div>
  );

  const back = () => {
    setError("");
    setStep(step - 1);
  };

  // 第 2 步可用条件：四条密码规则全过 + 路径合法 + 用户名非空。
  const accountOK = rulesOK && pErr === "" && draft.username.trim().length > 0;
  // 第 3 步可用条件：不启用缓存，或字段齐、且当前 DSN 刚测试通过。
  const currentDSN = buildDSN(draft);
  const testFresh = tested !== null && tested.dsn === currentDSN && tested.ok;
  const storageOK = !draft.storageOn || (dbFieldsError(draft) === "" && testFresh);

  return (
    <div className="card">
      <h1>novel-proxy 初始化</h1>
      {stepsHeader}
      {error ? <p className="error">{error}</p> : null}

      {step === 1 ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await post("/token", { token });
              setStep(2);
            });
          }}
        >
          <label className="field">
            <span className="field-label">初始化令牌</span>
            <input
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="启动日志或 data/init-token.txt 中的令牌"
              spellCheck={false}
            />
          </label>
          <div className="btn-row">
            <span />
            <button type="submit" className="btn primary" disabled={busy || token.trim() === ""}>
              下一步
            </button>
          </div>
        </form>
      ) : null}

      {step === 2 ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await post("/account", {
                username: draft.username,
                password: draft.password,
                admin_path: draft.adminPath,
              });
              setStep(3);
            });
          }}
        >
          <label className="field">
            <span className="field-label">管理员用户名</span>
            <input
              value={draft.username}
              onChange={(e) => setDraft({ ...draft, username: e.target.value })}
              spellCheck={false}
              autoComplete="username"
            />
          </label>
          <label className="field">
            <span className="field-label">密码</span>
            <span className="pw-row">
              <input
                type={draft.showPassword ? "text" : "password"}
                value={draft.password}
                onChange={(e) => setDraft({ ...draft, password: e.target.value })}
                autoComplete="new-password"
              />
              <button
                type="button"
                className="btn ghost pw-toggle"
                onClick={() => setDraft({ ...draft, showPassword: !draft.showPassword })}
              >
                {draft.showPassword ? "隐藏" : "显示"}
              </button>
            </span>
          </label>
          <ul className="rules">
            {rules.map((r) => (
              <li key={r.label} className={r.ok ? "rule ok" : "rule bad"}>
                <span className="rule-mark">{r.ok ? "已满足" : "未满足"}</span>
                {r.label}
              </li>
            ))}
          </ul>
          <label className="field">
            <span className="field-label">管理端路径</span>
            <input
              value={draft.adminPath}
              onChange={(e) => setDraft({ ...draft, adminPath: e.target.value })}
              spellCheck={false}
            />
          </label>
          {pErr ? <p className="field-err">{pErr}</p> : null}
          <p className="muted">管理端完整地址：{adminURL}</p>
          <div className="btn-row">
            <button type="button" className="btn ghost" onClick={back}>
              上一步
            </button>
            <button type="submit" className="btn primary" disabled={busy || !accountOK}>
              下一步
            </button>
          </div>
        </form>
      ) : null}

      {step === 3 ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await post("/storage", { enabled: draft.storageOn });
              if (draft.storageOn) {
                await post("/database", { driver: draft.driver, dsn: currentDSN });
              }
              setStep(4);
            });
          }}
        >
          <label className="check-row">
            <input
              type="checkbox"
              checked={draft.storageOn}
              onChange={(e) => setDraft({ ...draft, storageOn: e.target.checked })}
            />
            启用文章缓存
          </label>
          {draft.storageOn ? (
            <>
              <label className="field">
                <span className="field-label">数据库驱动</span>
                <select
                  value={draft.driver}
                  onChange={(e) => {
                    const v = e.target.value;
                    setDraft((d) => ({
                      ...d,
                      driver: v,
                      // 端口随驱动自动填默认值；sqlite 填默认文件路径。
                      db: { ...d.db, port: DEFAULT_PORT[v] ?? "" },
                      path: v === "sqlite" && d.path === "" ? SQLITE_DEFAULT_PATH : d.path,
                    }));
                  }}
                >
                  <option value="">请选择</option>
                  <option value="sqlite">sqlite</option>
                  <option value="postgres">postgres</option>
                  <option value="mysql">mysql</option>
                  <option value="mongo">mongo</option>
                </select>
              </label>
              {draft.driver === "sqlite" ? (
                <label className="field">
                  <span className="field-label">文件路径</span>
                  <input
                    value={draft.path}
                    onChange={(e) => setDraft({ ...draft, path: e.target.value })}
                    spellCheck={false}
                  />
                </label>
              ) : null}
              {draft.driver !== "" && draft.driver !== "sqlite" ? (
                <>
                  <label className="field">
                    <span className="field-label">主机</span>
                    <input
                      value={draft.db.host}
                      onChange={(e) => setDraft({ ...draft, db: { ...draft.db, host: e.target.value } })}
                      spellCheck={false}
                    />
                  </label>
                  <label className="field">
                    <span className="field-label">端口</span>
                    <input
                      value={draft.db.port}
                      onChange={(e) => setDraft({ ...draft, db: { ...draft.db, port: e.target.value } })}
                      spellCheck={false}
                    />
                  </label>
                  <label className="field">
                    <span className="field-label">用户名</span>
                    <input
                      value={draft.db.user}
                      onChange={(e) => setDraft({ ...draft, db: { ...draft.db, user: e.target.value } })}
                      spellCheck={false}
                      autoComplete="off"
                    />
                  </label>
                  <label className="field">
                    <span className="field-label">密码</span>
                    <span className="pw-row">
                      <input
                        type={draft.db.showPassword ? "text" : "password"}
                        value={draft.db.password}
                        onChange={(e) => setDraft({ ...draft, db: { ...draft.db, password: e.target.value } })}
                        autoComplete="new-password"
                      />
                      <button
                        type="button"
                        className="btn ghost pw-toggle"
                        onClick={() => setDraft({ ...draft, db: { ...draft.db, showPassword: !draft.db.showPassword } })}
                      >
                        {draft.db.showPassword ? "隐藏" : "显示"}
                      </button>
                    </span>
                  </label>
                  <label className="field">
                    <span className="field-label">数据库名</span>
                    <input
                      value={draft.db.dbName}
                      onChange={(e) => setDraft({ ...draft, db: { ...draft.db, dbName: e.target.value } })}
                      spellCheck={false}
                    />
                  </label>
                  {draft.driver === "mongo" ? (
                    <label className="field">
                      <span className="field-label">authSource</span>
                      <input
                        value={draft.db.authSource}
                        onChange={(e) => setDraft({ ...draft, db: { ...draft.db, authSource: e.target.value } })}
                        spellCheck={false}
                      />
                    </label>
                  ) : null}
                </>
              ) : null}
              {draft.driver !== "" ? (
                <>
                  <label className="field">
                    <span className="field-label">最终 DSN（密码已打码，可选中复制）</span>
                    <input className="dsn-preview" readOnly value={buildDSNMasked(draft)} spellCheck={false} />
                  </label>
                  <div className="test-row">
                    <button
                      type="button"
                      className="btn ghost"
                      disabled={busy}
                      onClick={() => {
                        const err = dbFieldsError(draft);
                        if (err) {
                          setTested({ dsn: currentDSN, ok: false, msg: err });
                          return;
                        }
                        run(async () => {
                          setTesting(true);
                          try {
                            const r = await post("/database/test", { driver: draft.driver, dsn: currentDSN });
                            setTested({ dsn: currentDSN, ok: !!r.ok, msg: String(r.message ?? "") });
                          } finally {
                            setTesting(false);
                          }
                        });
                      }}
                    >
                      {testing ? "测试中..." : "测试连接"}
                    </button>
                    {tested !== null && tested.dsn === currentDSN ? (
                      <span className={"test-result " + (tested.ok ? "ok" : "err")}>
                        {tested.ok ? tested.msg || "连接成功" : tested.msg || "测试失败"}
                      </span>
                    ) : null}
                  </div>
                </>
              ) : null}
            </>
          ) : null}
          <div className="btn-row">
            <button type="button" className="btn ghost" onClick={back}>
              上一步
            </button>
            <button type="submit" className="btn primary" disabled={busy || !storageOK}>
              下一步
            </button>
          </div>
        </form>
      ) : null}

      {step === 4 ? (
        <div>
          <ul className="summary">
            <li>
              <span>管理员用户名</span>
              <b>{draft.username}</b>
            </li>
            <li>
              <span>密码</span>
              <b>●●●●●●●●</b>
            </li>
            <li>
              <span>管理端路径</span>
              <b>/{draft.adminPath}</b>
            </li>
            <li>
              <span>管理端地址</span>
              <b>{adminURL}</b>
            </li>
            <li>
              <span>文章缓存</span>
              <b>{draft.storageOn ? "启用" : "不启用"}</b>
            </li>
            {draft.storageOn ? (
              <>
                <li>
                  <span>数据库驱动</span>
                  <b>{draft.driver}</b>
                </li>
                <li>
                  <span>DSN</span>
                  <b>{buildDSNMasked(draft)}</b>
                </li>
              </>
            ) : null}
          </ul>
          <p className="effective">提交后立即生效，不需要重启</p>
          <div className="btn-row">
            <button type="button" className="btn ghost" onClick={back}>
              上一步
            </button>
            <button
              type="button"
              className="btn primary"
              disabled={busy}
              onClick={() =>
                run(async () => {
                  const d = await post("/confirm");
                  setDraft((prev) => ({ ...prev, adminPath: d.admin_path }));
                  setDone(true);
                })
              }
            >
              确认初始化
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

createRoot(document.getElementById("root")!).render(<Wizard />);
