import { useEffect, useState } from "react";
import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import type { Me } from "./api";
import { me } from "./api";
import { AdminLayout } from "./Layout";
import { LoginPage } from "./pages/Login";
import { StatsPage } from "./pages/StatsPage";
import { CachePage } from "./pages/CachePage";
import { RulesPage } from "./pages/RulesPage";
import { AuditPage } from "./pages/AuditPage";
import { OverviewPage } from "./pages/Overview";
import { AccessPage, FlagsPage, RateLimitsPage } from "./pages/access";
import {
  AccountPage,
  AccountPasswordPage,
  ConfigLogPage,
  ConfigUpstreamPage,
} from "./pages/account-config";

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
            <Route path="flags" element={<FlagsPage />} />
            <Route path="rate-limits" element={<RateLimitsPage />} />
            <Route path="access" element={<AccessPage />} />
            <Route path="account" element={meInfo ? <AccountPage me={meInfo} /> : null} />
            <Route path="account/password" element={<AccountPasswordPage />} />
            <Route path="config/log" element={<ConfigLogPage />} />
            <Route path="config/upstream" element={<ConfigUpstreamPage />} />
            <Route path="stats" element={<StatsPage />} />
            <Route path="cache" element={<CachePage />} />
            <Route path="rules" element={<RulesPage />} />
            <Route path="audit" element={<AuditPage />} />
          </Route>
        </Routes>
      </HashRouter>
    </ConfigProvider>
  );
}
