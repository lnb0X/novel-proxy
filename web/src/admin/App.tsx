import { useEffect, useState } from "react";
import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import type { Me } from "./api";
import { me } from "./api";
import { AdminLayout } from "./Layout";
import { LoginPage } from "./pages/Login";
import { OverviewPage } from "./pages/Overview";
import { PlaceholderPage } from "./pages/Placeholder";

// 视觉约束见 AGENTS.md：主色不用 indigo/violet，圆角统一 6px。
const theme = {
  token: {
    colorPrimary: "#1677ff",
    borderRadius: 6,
  },
};

export function AdminApp() {
  // 认证状态：加载时探测一次 /auth/me；401 落到登录页。
  const [meInfo, setMeInfo] = useState<Me | null>(null);
  const [checked, setChecked] = useState(false);

  useEffect(() => {
    me()
      .then(setMeInfo)
      .catch(() => setMeInfo(null))
      .finally(() => setChecked(true));
  }, []);

  if (!checked) {
    return null;
  }

  return (
    <ConfigProvider locale={zhCN} theme={theme}>
      <HashRouter>
        <Routes>
          <Route
            path="/login"
            element={
              meInfo ? (
                <Navigate to="/" replace />
              ) : (
                <LoginPage onLogin={(m) => setMeInfo(m)} />
              )
            }
          />
          <Route
            path="*"
            element={
              meInfo ? (
                <AdminLayout me={meInfo} onLogout={() => setMeInfo(null)} />
              ) : (
                <Navigate to="/login" replace />
              )
            }
          >
            <Route index element={<OverviewPage />} />
            <Route path="*" element={<PlaceholderPage />} />
          </Route>
        </Routes>
      </HashRouter>
    </ConfigProvider>
  );
}
