import { Layout, Menu, Typography } from "antd";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import type { Me } from "./api";
import { logout } from "./api";

const { Sider, Content, Header } = Layout;

// 侧边栏分组，路由与 CONTRACTS / BACKLOG 的十四个页面对应；
// 本期只实现 /login 与 /，其余为占位页。
const menuGroups: { group: string; items: { key: string; label: string }[] }[] = [
  { group: "总览", items: [{ key: "/", label: "概览" }] },
  {
    group: "访问与安全",
    items: [
      { key: "/flags", label: "接口开关" },
      { key: "/rate-limits", label: "限速" },
      { key: "/access", label: "IP 过滤" },
    ],
  },
  {
    group: "运行",
    items: [
      { key: "/stats", label: "统计" },
      { key: "/cache", label: "缓存" },
      { key: "/frontends", label: "前端版本" },
    ],
  },
  { group: "内容", items: [{ key: "/rules", label: "内容规则" }] },
  {
    group: "配置",
    items: [
      { key: "/config/log", label: "日志配置" },
      { key: "/config/upstream", label: "上游配置" },
    ],
  },
  {
    group: "账号与审计",
    items: [
      { key: "/account", label: "账号" },
      { key: "/account/password", label: "修改密码" },
      { key: "/audit", label: "审计" },
    ],
  },
];

export function AdminLayout(props: { me: Me; onLogout: () => void }) {
  const navigate = useNavigate();
  const location = useLocation();

  const menuItems = menuGroups.map((g) => ({
    key: g.group,
    label: g.group,
    type: "group" as const,
    children: g.items.map((it) => ({ key: it.key, label: it.label })),
  }));

  const onMenuClick = (info: { key: string }) => {
    navigate(info.key);
  };

  const onLogout = async () => {
    try {
      await logout();
    } finally {
      props.onLogout();
      navigate("/login", { replace: true });
    }
  };

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider width={200} theme="light" breakpoint="lg" collapsedWidth="0">
        <div style={{ padding: "16px 16px 8px" }}>
          <Typography.Title level={5} style={{ margin: 0 }}>
            novel-proxy
          </Typography.Title>
        </div>
        <Menu
          mode="inline"
          items={menuItems}
          onClick={onMenuClick}
          selectedKeys={[location.pathname]}
          style={{ borderInlineEnd: "none" }}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            background: "#fff",
            padding: "0 24px",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: "1px solid #f0f0f0",
            height: 48,
          }}
        >
          <Typography.Text type="secondary">
            {props.me.username}（{props.me.perm}）
          </Typography.Text>
          <Typography.Link onClick={onLogout}>退出登录</Typography.Link>
        </Header>
        <Content style={{ padding: 24 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
