import { useState } from "react";
import { Alert, Button, Form, Input, Typography } from "antd";
import { useNavigate } from "react-router-dom";
import { ApiError, UnauthorizedError, login, type Me } from "../api";

export function LoginPage(props: { onLogin: (me: Me) => void }) {
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true);
    setError("");
    try {
      await login(values.username, values.password);
      const info = await (await import("../api")).me();
      props.onLogin(info);
      navigate("/", { replace: true });
    } catch (e) {
      if (e instanceof ApiError) {
        setError(e.message);
      } else if (e instanceof UnauthorizedError) {
        setError("登录已过期，请重试");
      } else {
        setError("网络错误，请重试");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#f5f5f5",
      }}
    >
      <div
        style={{
          background: "#fff",
          padding: 32,
          borderRadius: 6,
          width: 360,
        }}
      >
        <Typography.Title level={4} style={{ marginTop: 0 }}>
          novel-proxy 管理端
        </Typography.Title>
        {error ? <Alert type="error" message={error} style={{ marginBottom: 16 }} /> : null}
        <Form layout="vertical" onFinish={onFinish}>
          <Form.Item
            name="username"
            label="用户名"
            rules={[{ required: true, message: "请输入用户名" }]}
          >
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: "请输入密码" }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登录
          </Button>
        </Form>
      </div>
    </div>
  );
}
