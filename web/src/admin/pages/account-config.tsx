import { useEffect, useState } from "react";
import { Alert, Button, Card, Descriptions, Form, Input, InputNumber, Select, Spin, Switch, Typography, message } from "antd";
import {
  changePassword,
  getLogConfig,
  getUpstreamConfig,
  getLogConfig as _getLogConfig,
  getUpstreamConfig as _getUpstreamConfig,
  putLogConfig,
  putUpstreamConfig,
  type LogConfig,
  type Me,
  type UpstreamConfig,
} from "../api";

void _getLogConfig;
void _getUpstreamConfig;

// /account：只读，数据来自 /auth/me。
export function AccountPage(props: { me: Me }) {
  return (
    <div>
      <Typography.Title level={4}>账号</Typography.Title>
      <Card size="small">
        <Descriptions column={1} size="small">
          <Descriptions.Item label="用户名">{props.me.username}</Descriptions.Item>
          <Descriptions.Item label="权限">{props.me.perm}</Descriptions.Item>
          <Descriptions.Item label="会话到期（exp）">
            {new Date(props.me.exp * 1000).toLocaleString()}
          </Descriptions.Item>
          <Descriptions.Item label="绝对上限（sat）">
            {new Date(props.me.sat * 1000).toLocaleString()}
          </Descriptions.Item>
        </Descriptions>
      </Card>
    </div>
  );
}

// /account/password：旧密码 + 新密码 + 确认。前端先做一致性校验，
// 复杂度策略以后端为准（40001 的提示会明确展示）。
export function AccountPasswordPage() {
  const [oldPw, setOldPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState("");
  const [ok, setOk] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setErr("");
    setOk("");
    if (newPw !== confirm) {
      setErr("两次输入的新密码不一致");
      return;
    }
    setBusy(true);
    try {
      await changePassword(oldPw, newPw);
      setOk("密码已修改，请重新登录");
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <Typography.Title level={4}>修改密码</Typography.Title>
      {err ? <Alert type="error" message={err} style={{ marginBottom: 12 }} /> : null}
      {ok ? <Alert type="success" message={ok} style={{ marginBottom: 12 }} /> : null}
      <Form layout="vertical" style={{ maxWidth: 420 }} onFinish={submit}>
        <Form.Item label="旧密码" required>
          <Input.Password value={oldPw} onChange={(e) => setOldPw(e.target.value)} />
        </Form.Item>
        <Form.Item label="新密码" required extra="至少 12 位，小写/大写/数字/符号至少三类，不得包含用户名">
          <Input.Password value={newPw} onChange={(e) => setNewPw(e.target.value)} />
        </Form.Item>
        <Form.Item label="确认新密码" required>
          <Input.Password value={confirm} onChange={(e) => setConfirm(e.target.value)} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={busy}>
          修改密码
        </Button>
      </Form>
    </div>
  );
}

// /config/log：五项。level 立即生效，滚动四项重启后生效（页面标注）。
export function ConfigLogPage() {
  const [c, setC] = useState<LogConfig | null>(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getLogConfig()
      .then(setC)
      .catch((e) => setErr(e.message));
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!c) return <Spin />;

  const save = async () => {
    setSaving(true);
    try {
      const saved = await putLogConfig(c);
      setC(saved);
      message.success("level 已立即生效；滚动四项已写盘，重启后生效");
    } catch (e) {
      message.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <Typography.Title level={4}>日志配置</Typography.Title>
      <Card size="small" style={{ maxWidth: 460 }}>
        <Form layout="vertical">
          <Form.Item label="level（立即生效）" required>
            <Select
              value={c.level}
              onChange={(v) => setC({ ...c, level: v })}
              options={[
                { value: "debug", label: "debug" },
                { value: "info", label: "info" },
                { value: "error", label: "error" },
              ]}
            />
          </Form.Item>
          <Form.Item label="max_size_mb（重启后生效）">
            <InputNumber min={1} value={c.max_size_mb} onChange={(v) => setC({ ...c, max_size_mb: v ?? 50 })} />
          </Form.Item>
          <Form.Item label="max_backups（重启后生效）">
            <InputNumber min={1} value={c.max_backups} onChange={(v) => setC({ ...c, max_backups: v ?? 14 })} />
          </Form.Item>
          <Form.Item label="max_age_days（重启后生效）">
            <InputNumber min={1} value={c.max_age_days} onChange={(v) => setC({ ...c, max_age_days: v ?? 14 })} />
          </Form.Item>
          <Form.Item label="compress（重启后生效）">
            <Switch checked={c.compress} onChange={(v) => setC({ ...c, compress: v })} />
          </Form.Item>
          <Button type="primary" loading={saving} onClick={save}>
            保存
          </Button>
        </Form>
      </Card>
    </div>
  );
}

// /config/upstream：timeout 与 max_concurrency，均立即生效。
export function ConfigUpstreamPage() {
  const [c, setC] = useState<UpstreamConfig | null>(null);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getUpstreamConfig()
      .then(setC)
      .catch((e) => setErr(e.message));
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!c) return <Spin />;

  const save = async () => {
    setSaving(true);
    try {
      const saved = await putUpstreamConfig(c);
      setC(saved);
      message.success("已保存并立即生效");
    } catch (e) {
      message.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div>
      <Typography.Title level={4}>上游配置</Typography.Title>
      <Card size="small" style={{ maxWidth: 420 }}>
        <Form layout="vertical">
          <Form.Item label="timeout_seconds（立即生效，1-300）" required>
            <InputNumber
              min={1}
              max={300}
              value={c.timeout_seconds}
              onChange={(v) => setC({ ...c, timeout_seconds: v ?? 10 })}
            />
          </Form.Item>
          <Form.Item label="max_concurrency（立即生效，1-1000）" required>
            <InputNumber
              min={1}
              max={1000}
              value={c.max_concurrency}
              onChange={(v) => setC({ ...c, max_concurrency: v ?? 8 })}
            />
          </Form.Item>
          <Button type="primary" loading={saving} onClick={save}>
            保存
          </Button>
        </Form>
      </Card>
    </div>
  );
}
