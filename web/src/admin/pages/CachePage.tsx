import { useEffect, useState } from "react";
import { Alert, Button, Card, Input, Modal, Table, Typography, message } from "antd";
import {
  deleteCacheAll,
  deleteCacheByBook,
  deleteCacheItem,
  getCacheList,
  type CacheListData,
} from "../api";

// deleteCacheByBook 在“按书清空”按钮里使用。


// /cache：搜索、分页、单条删、按书清空、全清（手输 DELETE 放行）。
export function CachePage() {
  const [data, setData] = useState<CacheListData | null>(null);
  const [err, setErr] = useState("");
  const [bookId, setBookId] = useState("");
  const [chapterId, setChapterId] = useState("");
  const [page, setPage] = useState(1);
  const [allOpen, setAllOpen] = useState(false);
  const [confirmText, setConfirmText] = useState("");

  const load = (p: number) => {
    getCacheList(bookId, chapterId, p, 20)
      .then((d) => {
        setData(d);
        setPage(p);
      })
      .catch((e) => setErr(e.message));
  };

  useEffect(() => {
    load(1);
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!data) return null;

  const wrap = (fn: () => Promise<unknown>, okMsg: string) => {
    fn()
      .then(() => {
        message.success(okMsg);
        load(page);
      })
      .catch((e) => message.error((e as Error).message));
  };

  return (
    <div>
      <Typography.Title level={4}>缓存</Typography.Title>
      <Card size="small" style={{ marginBottom: 12 }}>
        <Input
          placeholder="book_id"
          style={{ width: 220, marginRight: 8 }}
          value={bookId}
          onChange={(e) => setBookId(e.target.value)}
        />
        <Input
          placeholder="chapter_id"
          style={{ width: 220, marginRight: 8 }}
          value={chapterId}
          onChange={(e) => setChapterId(e.target.value)}
        />
        <Button
          onClick={() => {
            load(1);
          }}
        >
          搜索
        </Button>
        <Button
          style={{ marginLeft: 8 }}
          onClick={() => {
            setBookId("");
            setChapterId("");
            load(1);
          }}
        >
          重置
        </Button>
        <Button
          danger
          style={{ marginLeft: 8 }}
          disabled={!bookId}
          onClick={() => wrap(() => deleteCacheByBook(bookId), "已按书清空")}
        >
          按书清空
        </Button>
        <Button danger style={{ marginLeft: 8 }} onClick={() => setAllOpen(true)}>
          全部清空
        </Button>
      </Card>
      <Card size="small" style={{ marginBottom: 12 }}>
        共 {data.total} 条 / {data.total_bytes} 字节 / 命中率 {data.hit_rate.toFixed(2)}
      </Card>
      <Table
        size="small"
        rowKey={(r) => r.item_id}
        dataSource={data.items}
        pagination={{
          current: page,
          pageSize: 20,
          total: data.total,
          onChange: (p) => load(p),
        }}
        columns={[
          { title: "item_id", dataIndex: "item_id" },
          { title: "book_id", dataIndex: "book_id" },
          { title: "标题", dataIndex: "title" },
          { title: "字节", dataIndex: "size_bytes" },
          {
            title: "创建时间",
            dataIndex: "created_at",
            render: (v: number) => new Date(v * 1000).toLocaleString(),
          },
          {
            title: "操作",
            render: (_: unknown, r) => (
              <Button
                size="small"
                danger
                onClick={() =>
                  wrap(() => deleteCacheItem(r.item_id), "已删除 " + r.item_id)
                }
              >
                删除
              </Button>
            ),
          },
        ]}
      />
      <Modal
        title="全部清空"
        open={allOpen}
        onCancel={() => setAllOpen(false)}
        footer={null}
      >
        <p>此操作删除全部文章缓存且不可恢复。输入 DELETE 以放行：</p>
        <Input value={confirmText} onChange={(e) => setConfirmText(e.target.value)} />
        <Button
          danger
          type="primary"
          block
          style={{ marginTop: 12 }}
          disabled={confirmText !== "DELETE"}
          onClick={() => {
            setAllOpen(false);
            setConfirmText("");
            wrap(() => deleteCacheAll(), "已全部清空");
          }}
        >
          全部清空
        </Button>
      </Modal>
    </div>
  );
}
