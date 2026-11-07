import { useEffect, useState } from "react";
import { Alert, Button, Card, Input, InputNumber, Popconfirm, Select, Space, Spin, Tag, Typography, message } from "antd";
import {
  getFlags,
  getIpFilter,
  getRateLimits,
  putIpFilter,
  putRateLimits,
  setFlag,
  type FlagSet,
  type IpFilter,
  type RateLimits,
} from "../api";

// /flags：四个开关，切换带二次确认。
export function FlagsPage() {
  const [flags, setFlags] = useState<FlagSet | null>(null);
  const [err, setErr] = useState("");

  const load = () =>
    getFlags()
      .then(setFlags)
      .catch((e) => setErr(e.message));

  useEffect(() => {
    load();
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!flags) return <Spin />;

  const rows = (["search", "detail", "catalog", "content"] as const).map((name) => (
    <Card key={name} size="small" style={{ marginBottom: 12 }}>
      <Space>
        <Typography.Text strong>{name}</Typography.Text>
        {flags[name] ? <Tag color="green">开启</Tag> : <Tag color="red">关闭</Tag>}
        <Popconfirm
          title={flags[name] ? "确认关闭该接口？" : "确认开启该接口？"}
          onConfirm={() =>
            setFlag(name, !flags[name])
              .then(load)
              .catch((e) => message.error(e.message))
          }
        >
          <Button size="small">{flags[name] ? "关闭" : "开启"}</Button>
        </Popconfirm>
      </Space>
    </Card>
  ));

  return (
    <div>
      <Typography.Title level={4}>接口开关</Typography.Title>
      {rows}
    </div>
  );
}

// /rate-limits：四行 ×（模式 / 每分钟 / 突发），整体保存。
export function RateLimitsPage() {
  const [rl, setRl] = useState<RateLimits | null>(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getRateLimits()
      .then(setRl)
      .catch((e) => setErr(e.message));
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!rl) return <Spin />;

  const names = ["search", "detail", "catalog", "content"] as const;
  const update = (name: (typeof names)[number], field: keyof RateLimits["search"], value: unknown) =>
    setRl({ ...rl, [name]: { ...rl[name], [field]: value } });

  const save = async () => {
    setSaving(true);
    try {
      const saved = await putRateLimits(rl);
      setRl(saved);
      message.success("已保存并立即生效");
    } catch (e) {
      message.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <Typography.Title level={4}>限速</Typography.Title>
      {names.map((name) => (
        <Card key={name} size="small" style={{ marginBottom: 12 }} title={name}>
          <Space wrap>
            <label>
              模式{" "}
              <Select
                value={rl[name].mode}
                style={{ width: 120 }}
                onChange={(v) => update(name, "mode", v)}
                options={[
                  { value: "off", label: "off" },
                  { value: "global", label: "global" },
                  { value: "per_ip", label: "per_ip" },
                ]}
              />
            </label>
            <label>
              每分钟{" "}
              <InputNumber
                min={0}
                value={rl[name].per_minute}
                onChange={(v) => update(name, "per_minute", v ?? 0)}
              />
            </label>
            <label>
              突发{" "}
              <InputNumber
                min={0}
                value={rl[name].burst}
                onChange={(v) => update(name, "burst", v ?? 0)}
              />
            </label>
          </Space>
        </Card>
      ))}
      <Button type="primary" loading={saving} onClick={save}>
        保存
      </Button>
    </div>
  );
}

// /access：IP 过滤，模式 + 范围 + 名单增删。
export function AccessPage() {
  const [f, setF] = useState<IpFilter | null>(null);
  const [err, setErr] = useState("");
  const [newRule, setNewRule] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getIpFilter()
      .then(setF)
      .catch((e) => setErr(e.message));
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!f) return <Spin />;

  const save = async (next: IpFilter, okMsg: string) => {
    setSaving(true);
    try {
      const saved = await putIpFilter(next);
      setF(saved);
      message.success(okMsg);
    } catch (e) {
      // 锁死检查等失败要明确提示，不静默。
      message.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <Typography.Title level={4}>IP 过滤</Typography.Title>
      <Card size="small" style={{ marginBottom: 12 }}>
        <Space wrap>
          <label>
            模式{" "}
            <Select
              value={f.mode}
              style={{ width: 110 }}
              onChange={(v) => setF({ ...f, mode: v })}
              options={[
                { value: "off", label: "off" },
                { value: "allow", label: "allow" },
                { value: "block", label: "block" },
              ]}
            />
          </label>
          <label>
            范围{" "}
            <Select
              value={f.scope}
              style={{ width: 110 }}
              onChange={(v) => setF({ ...f, scope: v })}
              options={[
                { value: "all", label: "all" },
                { value: "api", label: "api" },
                { value: "admin", label: "admin" },
              ]}
            />
          </label>
          <Button loading={saving} onClick={() => save(f, "已保存")}>
            保存模式与范围
          </Button>
        </Space>
      </Card>
      <Card title="名单（CIDR 或裸 IP，最多 200 条）" size="small">
        {f.rules.length === 0 ? <Typography.Text type="secondary">空</Typography.Text> : null}
        {f.rules.map((r) => (
          <div key={r} style={{ marginBottom: 6 }}>
            <Tag>{r}</Tag>
            <Button
              size="small"
              danger
              onClick={() => setF({ ...f, rules: f.rules.filter((x) => x !== r) })}
            >
              删除
            </Button>
          </div>
        ))}
        <Space.Compact style={{ marginTop: 8 }}>
          <Input
            placeholder="10.0.0.0/8 或 1.2.3.4"
            value={newRule}
            onChange={(e) => setNewRule(e.target.value)}
          />
          <Button
            onClick={() => {
              if (!newRule.trim()) {
                message.warning("规则不能为空");
                return;
              }
              if (f.rules.includes(newRule.trim())) {
                message.warning("规则已存在");
                return;
              }
              setF({ ...f, rules: [...f.rules, newRule.trim()] });
              setNewRule("");
            }}
          >
            添加
          </Button>
        </Space.Compact>
        <div style={{ marginTop: 12 }}>
          <Button
            type="primary"
            loading={saving}
            onClick={() => {
              if (f.mode === "allow") {
                message.warning("allow 模式下请确认当前 IP 在名单内，否则保存会被服务端拒绝");
              }
              save(f, "名单已保存");
            }}
          >
            保存名单
          </Button>
        </div>
      </Card>
    </div>
  );
}
