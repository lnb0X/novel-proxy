import { useEffect, useState, type ReactNode } from "react";
import { Card, Descriptions, Spin, Tag, Typography } from "antd";
import { getFlags, getFrontends, getIpFilter, type AppFrontends, type FlagSet, type IpFilter } from "../api";

// 概览页：聚合当前可用的管理接口数据。
// /stats/overview 属于 W8（已挂起），落地后可在此页补充请求统计。
export function OverviewPage() {
  const [flags, setFlags] = useState<FlagSet | null | undefined>(undefined);
  const [ipf, setIpf] = useState<IpFilter | null | undefined>(undefined);
  const [fe, setFe] = useState<{ reader: AppFrontends; admin: AppFrontends } | null | undefined>(undefined);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // 各端点独立加载：单个接口缺失（如开关接口未实现）不影响其他面板。
    // undefined = 加载中，null = 接口不可用。
    getFlags().then(setFlags).catch(() => setFlags(null));
    getIpFilter().then(setIpf).catch(() => setIpf(null));
    getFrontends().then(setFe).catch(() => setFe(null));
    Promise.all([
      getIpFilter().catch(() => null),
      getFrontends().catch(() => null),
    ]).finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <Spin />;
  }

  const render = (v: unknown, node: ReactNode) =>
    v === null ? <Tag>接口暂不可用</Tag> : node;

  const flagText = (v: boolean | undefined) =>
    v === undefined ? "未知" : v ? "开启" : "关闭";

  return (
    <div>
      <Typography.Title level={4}>概览</Typography.Title>
      <Card title="接口开关" size="small" style={{ marginBottom: 16 }}>
        {render(flags, (
          <Descriptions size="small" column={4}>
            <Descriptions.Item label="search">
              <Tag color={flags?.search ? "green" : "red"}>{flagText(flags?.search)}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="detail">
              <Tag color={flags?.detail ? "green" : "red"}>{flagText(flags?.detail)}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="catalog">
              <Tag color={flags?.catalog ? "green" : "red"}>{flagText(flags?.catalog)}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="content">
              <Tag color={flags?.content ? "green" : "red"}>{flagText(flags?.content)}</Tag>
            </Descriptions.Item>
          </Descriptions>
        ))}
      </Card>
      <Card title="IP 过滤" size="small" style={{ marginBottom: 16 }}>
        {render(ipf, (
          <Descriptions size="small" column={3}>
            <Descriptions.Item label="模式">{ipf?.mode}</Descriptions.Item>
            <Descriptions.Item label="范围">{ipf?.scope}</Descriptions.Item>
            <Descriptions.Item label="规则条数">{ipf?.rules.length ?? 0}</Descriptions.Item>
          </Descriptions>
        ))}
      </Card>
      <Card title="前端版本" size="small">
        {render(fe, (
          <Descriptions size="small" column={2}>
            <Descriptions.Item label="reader 当前">
              {fe?.reader.current || "未部署"}
            </Descriptions.Item>
            <Descriptions.Item label="admin 当前">
              {fe?.admin.current || "未部署"}
            </Descriptions.Item>
          </Descriptions>
        ))}
      </Card>
    </div>
  );
}
