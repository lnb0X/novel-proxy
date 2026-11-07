import { useEffect, useState } from "react";
import { Alert, Card, Col, DatePicker, Row, Spin, Statistic, Table, Tag, Typography } from "antd";
import { getStatsOverview, getStatsQuery, type StatsOverview, type StatsQuery } from "../api";

const { RangePicker } = DatePicker;

// /stats：概览 + 时间范围筛选（小时桶趋势、分位数、错误码、热门词与书）。
export function StatsPage() {
  const [ov, setOv] = useState<StatsOverview | null>(null);
  const [q, setQ] = useState<StatsQuery | null>(null);
  const [err, setErr] = useState("");
  const [range, setRange] = useState<[string, string]>(["", ""]);

  useEffect(() => {
    getStatsOverview()
      .then(setOv)
      .catch((e) => setErr(e.message));
    loadQuery("", "");
  }, []);

  const loadQuery = (from: string, to: string) => {
    getStatsQuery(from, to, "")
      .then(setQ)
      .catch((e) => setErr(e.message));
  };

  if (err) return <Alert type="error" message={err} />;
  if (!ov || !q) return <Spin />;

  const healthColor =
    ov.upstream_health === "ok" ? "green" : ov.upstream_health === "degraded" ? "orange" : "red";

  return (
    <div>
      <Typography.Title level={4}>统计</Typography.Title>
      <Row gutter={12} style={{ marginBottom: 16 }}>
        <Col><Statistic title="今日请求" value={ov.today_requests} /></Col>
        <Col><Statistic title="今日错误" value={ov.today_errors} /></Col>
        <Col><Statistic title="P95 (ms)" value={ov.p95_ms} /></Col>
        <Col><Statistic title="缓存命中率" value={ov.cache_hit_rate} precision={2} /></Col>
        <Col>
          <Statistic
            title="上游健康"
            valueRender={() => <Tag color={healthColor}>{ov.upstream_health}</Tag>}
          />
        </Col>
      </Row>

      <Card title="时间范围" size="small" style={{ marginBottom: 16 }}>
        <RangePicker
          showTime
          onChange={(dates) => {
            if (!dates || dates.length !== 2) {
              setRange(["", ""]);
              loadQuery("", "");
              return;
            }
            const from = dates[0]!.toISOString();
            const to = dates[1]!.toISOString();
            setRange([from, to]);
            loadQuery(from, to);
          }}
        />
        <Typography.Text type="secondary" style={{ marginLeft: 12 }}>
          当前筛选：{range[0] || "不限"} ~ {range[1] || "不限"}
        </Typography.Text>
      </Card>

      <Card title="小时桶" size="small" style={{ marginBottom: 16 }}>
        <Table
          size="small"
          rowKey={(r) => String(r.time)}
          dataSource={q.buckets}
          pagination={false}
          columns={[
            { title: "小时(UTC)", dataIndex: "time", render: (v: number) => new Date(v * 1000).toLocaleString() },
            { title: "请求数", dataIndex: "requests" },
            { title: "错误数", dataIndex: "errors" },
          ]}
        />
      </Card>

      <Row gutter={12}>
        <Col span={8}>
          <Card title="分位数 (ms)" size="small" style={{ marginBottom: 16 }}>
            <Statistic title="P50" value={q.p50} />
            <Statistic title="P95" value={q.p95} />
            <Statistic title="P99" value={q.p99} />
          </Card>
        </Col>
        <Col span={8}>
          <Card title="错误码分布" size="small" style={{ marginBottom: 16 }}>
            {Object.keys(q.error_codes).length === 0 ? (
              <Typography.Text type="secondary">无</Typography.Text>
            ) : (
              Object.entries(q.error_codes).map(([code, n]) => (
                <p key={code}>
                  <Tag color="red">{code}</Tag> {n} 次
                </p>
              ))
            )}
          </Card>
        </Col>
        <Col span={8}>
          <Card title="热门搜索词 / 热门书" size="small" style={{ marginBottom: 16 }}>
            {q.top_queries.map((t) => (
              <p key={t.name}>
                {t.name}（{t.count}）
              </p>
            ))}
            {q.top_books.map((t) => (
              <p key={t.name}>
                book {t.name}（{t.count}）
              </p>
            ))}
          </Card>
        </Col>
      </Row>

      <Card title="最近错误" size="small">
        <Table
          size="small"
          rowKey={(r) => r.trace_id + r.path}
          dataSource={ov.recent_errors}
          pagination={false}
          columns={[
            { title: "时间", dataIndex: "time", render: (v: number) => new Date(v * 1000).toLocaleString() },
            { title: "路径", dataIndex: "path" },
            { title: "状态", dataIndex: "status" },
            { title: "业务码", dataIndex: "code" },
            { title: "trace_id", dataIndex: "trace_id" },
          ]}
        />
      </Card>
    </div>
  );
}
