import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Popconfirm, Spin, Table, Tag, Typography, Upload, message } from "antd";
import {
  activateFrontend,
  confirmFrontend,
  deleteFrontend,
  getFrontendsAll,
  rollbackFrontend,
  uploadFrontend,
  type FrontendApp,
  type FrontendVersion,
} from "../api";

// /frontends：列出 reader / admin 的当前版本与历史版本，
// 上传 zip、激活（10 秒确认浮层）、确认、回滚、删除。
export function FrontendsPage() {
  const [data, setData] = useState<{ reader: FrontendApp; admin: FrontendApp } | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  // 每次激活启动 10 秒倒计时；倒计时归零且未确认时提示已自动回滚。
  const [countdown, setCountdown] = useState<{ app: "reader" | "admin"; version: string; left: number } | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  function load() {
    getFrontendsAll()
      .then(setData)
      .catch((e) => setErr(e.message));
  }

  useEffect(() => {
    load();
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!data) return <Spin />;

  const wrap = (fn: () => Promise<unknown>, okMsg: string) => {
    setBusy(true);
    fn()
      .then(() => {
        message.success(okMsg);
        load();
      })
      .catch((e) => message.error((e as Error).message))
      .finally(() => setBusy(false));
  };

  const doUpload = (app: "reader" | "admin", file: File) => {
    setBusy(true);
    uploadFrontend(app, file)
      .then((v) => {
        message.success("上传成功：" + v.version);
        load();
      })
      .catch((e) => message.error((e as Error).message))
      .finally(() => setBusy(false));
  };

  const doActivate = (app: "reader" | "admin", version: string) => {
    setBusy(true);
    activateFrontend(app, version)
      .then(() => {
        load();
        // 10 秒确认倒计时浮层。
        setCountdown({ app, version, left: 10 });
        if (timerRef.current) clearInterval(timerRef.current);
        timerRef.current = setInterval(() => {
          setCountdown((c) => {
            if (!c) return null;
            if (c.left <= 1) {
              clearInterval(timerRef.current!);
              message.info(app + " 未在时限内确认，已自动回滚到上一版本");
              load();
              return null;
            }
            return { ...c, left: c.left - 1 };
          });
        }, 1000);
      })
      .catch((e) => message.error((e as Error).message))
      .finally(() => setBusy(false));
  };

  const doConfirm = (app: "reader" | "admin", version: string) => {
    if (timerRef.current) clearInterval(timerRef.current);
    setCountdown(null);
    wrap(() => confirmFrontend(app, version), "已确认 " + version);
  };

  const appCard = (app: "reader" | "admin") => {
    const a: FrontendApp = data[app];
    return (
      <Card
        title={app}
        size="small"
        style={{ marginBottom: 16 }}
        extra={
          <Upload
            beforeUpload={(file) => {
              doUpload(app, file);
              return false; // 阻止 antd 自动上传
            }}
            showUploadList={false}
          >
            <Button size="small" loading={busy}>
              上传 zip
            </Button>
          </Upload>
        }
      >
        <p>
          当前：<Tag color="blue">{a.current || "无"}</Tag>
          {a.previous ? (
            <>
              上一版本：<Tag>{a.previous}</Tag>
              <Popconfirm title="确认回滚到上一版本？" onConfirm={() => wrap(() => rollbackFrontend(app), "已回滚")}>
                <Button size="small">回滚</Button>
              </Popconfirm>
            </>
          ) : null}
        </p>
        <Table
          size="small"
          rowKey={(r) => r.version}
          scroll={{ x: "max-content" }}
          dataSource={a.versions}
          pagination={false}
          columns={[
            { title: "版本", dataIndex: "version" },
            { title: "字节", dataIndex: "size" },
            { title: "文件数", dataIndex: "files" },
            {
              title: "创建时间",
              dataIndex: "created_at",
              render: (v: number) => new Date(v * 1000).toLocaleString(),
            },
            {
              title: "操作",
              render: (_: unknown, r: FrontendVersion) => (
                <Button size="small" onClick={() => doActivate(app, r.version)}>
                  激活
                </Button>
              ),
            },
            {
              title: "删除",
              render: (_: unknown, r: FrontendVersion) => (
                <Popconfirm
                  title="确认删除该版本？"
                  onConfirm={() => wrap(() => deleteFrontend(app, r.version), "已删除")}
                >
                  <Button size="small" danger>
                    删除
                  </Button>
                </Popconfirm>
              ),
            },
          ]}
        />
      </Card>
    );
  };

  return (
    <div>
      <Typography.Title level={4}>前端版本管理</Typography.Title>
      {countdown ? (
        <Alert
          type="warning"
          message={countdown.app + " 已激活 " + countdown.version + "，" + countdown.left + " 秒后未确认将自动回滚"}
          action={
            <Button size="small" onClick={() => doConfirm(countdown.app, countdown.version)}>
              立即确认
            </Button>
          }
          style={{ marginBottom: 12 }}
        />
      ) : null}
      {appCard("reader")}
      {appCard("admin")}
    </div>
  );
}
