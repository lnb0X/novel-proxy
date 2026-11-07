import { useEffect, useState } from "react";
import { Button, Card, DatePicker, Select, Table, Typography } from "antd";
import { getAudit, type AuditPageData } from "../api";

// /audit：表格 + 按事件类型与时间筛选 + 分页。
export function AuditPage() {
  const [data, setData] = useState<AuditPageData | null>(null);
  const [event, setEvent] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [page, setPage] = useState(1);
  const size = 20;

  const load = (p: number) => {
    getAudit(event, from, to, p, size)
      .then((d) => {
        setData(d);
        setPage(p);
      })
      .catch(() => undefined);
  };

  useEffect(() => {
    load(1);
  }, []);

  return (
    <div>
      <Typography.Title level={4}>审计</Typography.Title>
      <Card size="small" style={{ marginBottom: 12 }}>
        <Select
          allowClear
          placeholder="事件类型"
          style={{ width: 200, marginRight: 8 }}
          value={event || undefined}
          onChange={(v) => setEvent(v ?? "")}
          options={[
            "login_success", "login_fail", "logout", "password_change", "force_logout",
            "flag_change", "rate_limit_change", "ip_filter_change", "cache_clear",
            "rules_change", "frontend_upload", "frontend_activate", "frontend_rollback",
            "frontend_delete", "config_change",
          ].map((e) => ({ value: e, label: e }))}
        />
        <DatePicker
          showTime
          placeholder="from"
          onChange={(d) => setFrom(d ? d.toISOString() : "")}
          style={{ marginRight: 8 }}
        />
        <DatePicker
          showTime
          placeholder="to"
          onChange={(d) => setTo(d ? d.toISOString() : "")}
          style={{ marginRight: 8 }}
        />
        <Button onClick={() => load(1)}>筛选</Button>
      </Card>
      <Table
        size="small"
        rowKey={(r) => r.ts + r.event + r.detail}
        dataSource={data?.items ?? []}
        loading={!data}
        pagination={{
          current: page,
          pageSize: size,
          total: data?.total ?? 0,
          onChange: (p) => load(p),
        }}
        columns={[
          { title: "时间", dataIndex: "ts", width: 180, render: (v: string) => new Date(v).toLocaleString() },
          { title: "事件", dataIndex: "event", width: 150 },
          { title: "操作者", dataIndex: "actor", width: 100 },
          { title: "IP", dataIndex: "ip", width: 130 },
          { title: "详情", dataIndex: "detail" },
          { title: "结果", dataIndex: "result", width: 100 },
        ]}
      />
    </div>
  );
}
