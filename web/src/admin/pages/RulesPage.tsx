import { useEffect, useState } from "react";
import { Alert, Button, Card, Checkbox, Input, Space, Spin, Tag, Typography, message } from "antd";
import { getContentRules, previewRules, putContentRules, type ContentRules } from "../api";

const emptySet = (): ContentRules["default"] => ({
  prepend: null,
  append: null,
  replace: [],
  override: null,
});

// /rules：全局三块 + 按书配置、replace 列表可上下移动、预览对比。
export function RulesPage() {
  const [cr, setCr] = useState<ContentRules | null>(null);
  const [err, setErr] = useState("");
  const [bookKey, setBookKey] = useState("");
  const [previewBook, setPreviewBook] = useState("");
  const [previewChapter, setPreviewChapter] = useState("");
  const [preview, setPreview] = useState<{
    original: string | null;
    result: string;
    applied: { rule: string; hits: number }[];
  } | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    getContentRules()
      .then(setCr)
      .catch((e) => setErr(e.message));
  }, []);

  if (err) return <Alert type="error" message={err} />;
  if (!cr) return <Spin />;

  const save = async (next: ContentRules, okMsg: string) => {
    setBusy(true);
    try {
      const saved = await putContentRules(next);
      setCr(saved);
      message.success(okMsg);
    } catch (e) {
      message.error((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  // replace 列表编辑：上移 / 下移 / 删除，顺序即执行顺序。
  const move = (
    list: { pattern: string; replacement: string; regex: boolean }[],
    setList: (l: ContentRules["default"]["replace"]) => void,
    i: number,
    dir: -1 | 1,
  ) => {
    const j = i + dir;
    if (j < 0 || j >= list.length) return;
    const next = [...list];
    [next[i], next[j]] = [next[j], next[i]];
    setList(next);
  };

  const replaceEditor = (
    list: ContentRules["default"]["replace"],
    setList: (l: ContentRules["default"]["replace"]) => void,
  ) => (
    <div>
      {list.map((r, i) => (
        <Space key={i} style={{ marginBottom: 6 }} wrap>
          <Input
            style={{ width: 200 }}
            value={r.pattern}
            placeholder="匹配内容"
            onChange={(e) => {
              const next = [...list];
              next[i] = { ...r, pattern: e.target.value };
              setList(next);
            }}
          />
          <Input
            style={{ width: 200 }}
            value={r.replacement}
            placeholder="替换为"
            onChange={(e) => {
              const next = [...list];
              next[i] = { ...r, replacement: e.target.value };
              setList(next);
            }}
          />
          <Checkbox
            checked={r.regex}
            onChange={(e) => {
              const next = [...list];
              next[i] = { ...r, regex: e.target.checked };
              setList(next);
            }}
          >
            正则
          </Checkbox>
          <Button size="small" disabled={i === 0} onClick={() => move(list, setList, i, -1)}>
            上移
          </Button>
          <Button size="small" disabled={i === list.length - 1} onClick={() => move(list, setList, i, 1)}>
            下移
          </Button>
          <Button size="small" danger onClick={() => setList(list.filter((_, j) => j !== i))}>
            删除
          </Button>
        </Space>
      ))}
      <Button
        size="small"
        onClick={() => setList([...list, { pattern: "", replacement: "", regex: false }])}
      >
        添加规则
      </Button>
    </div>
  );

  const globalEditor = (
    <Card title="全局规则" size="small" style={{ marginBottom: 12 }}>
      <Typography.Paragraph>
        prepend（响应头部拼接）
        <Input
          value={cr.default.prepend ?? ""}
          onChange={(e) =>
            setCr({ ...cr, default: { ...cr.default, prepend: e.target.value === "" ? null : e.target.value } })
          }
        />
      </Typography.Paragraph>
      <Typography.Paragraph>
        append（响应尾部拼接）
        <Input
          value={cr.default.append ?? ""}
          onChange={(e) =>
            setCr({ ...cr, default: { ...cr.default, append: e.target.value === "" ? null : e.target.value } })
          }
        />
      </Typography.Paragraph>
      <Typography.Paragraph>replace（级联，顺序即执行顺序）</Typography.Paragraph>
      {replaceEditor(cr.default.replace, (l) => setCr({ ...cr, default: { ...cr.default, replace: l } }))}
      <Typography.Paragraph style={{ marginTop: 12 }}>
        override（非空即全站接管）
        <Input
          value={cr.default.override ?? ""}
          onChange={(e) =>
            setCr({ ...cr, default: { ...cr.default, override: e.target.value === "" ? null : e.target.value } })
          }
        />
      </Typography.Paragraph>
      <Button type="primary" loading={busy} onClick={() => save(cr, "全局规则已保存并生效")}>
        保存全局规则
      </Button>
    </Card>
  );

  const bookEditor = (
    <Card title="按书配置" size="small" style={{ marginBottom: 12 }}>
      <Space style={{ marginBottom: 12 }} wrap>
        <Input
          placeholder="book_id"
          value={bookKey}
          style={{ width: 240 }}
          onChange={(e) => setBookKey(e.target.value)}
        />
        <Button
          onClick={() => {
            if (!bookKey.trim()) {
              message.warning("book_id 不能为空");
              return;
            }
            if (cr.books[bookKey.trim()]) {
              message.warning("该书已有配置");
              return;
            }
            setCr({ ...cr, books: { ...cr.books, [bookKey.trim()]: emptySet() } });
            setBookKey("");
          }}
        >
          添加按书配置
        </Button>
      </Space>
      {Object.keys(cr.books).map((id) => {
        const rs = cr.books[id];
        return (
          <Card
            key={id}
            type="inner"
            title={"book " + id}
            size="small"
            style={{ marginBottom: 12 }}
            extra={
              <Button
                size="small"
                danger
                onClick={() => {
                  const books = { ...cr.books };
                  delete books[id];
                  setCr({ ...cr, books });
                }}
              >
                删除配置
              </Button>
            }
          >
            <Typography.Paragraph>
              prepend（null 继承全局，空串抑制全局）
              <Input
                value={rs.prepend ?? ""}
                placeholder="（空 = 继承全局）"
                onChange={(e) =>
                  setCr({
                    ...cr,
                    books: { ...cr.books, [id]: { ...rs, prepend: e.target.value === "" ? null : e.target.value } },
                  })
                }
              />
            </Typography.Paragraph>
            <Typography.Paragraph>
              append
              <Input
                value={rs.append ?? ""}
                placeholder="（空 = 继承全局）"
                onChange={(e) =>
                  setCr({
                    ...cr,
                    books: { ...cr.books, [id]: { ...rs, append: e.target.value === "" ? null : e.target.value } },
                  })
                }
              />
            </Typography.Paragraph>
            <Typography.Paragraph>replace（全局之后追加执行）</Typography.Paragraph>
            {replaceEditor(rs.replace, (l) =>
              setCr({ ...cr, books: { ...cr.books, [id]: { ...rs, replace: l } } }),
            )}
            <Typography.Paragraph style={{ marginTop: 12 }}>
              override（非空即接管本书全部章节）
              <Input
                value={rs.override ?? ""}
                onChange={(e) =>
                  setCr({
                    ...cr,
                    books: { ...cr.books, [id]: { ...rs, override: e.target.value === "" ? null : e.target.value } },
                  })
                }
              />
            </Typography.Paragraph>
            <Button
              type="primary"
              loading={busy}
              onClick={() => save({ ...cr, books: { ...cr.books, [id]: rs } }, "book " + id + " 已保存")}
            >
              保存本书
            </Button>
          </Card>
        );
      })}
    </Card>
  );

  const previewPanel = (
    <Card title="预览" size="small">
      <Space style={{ marginBottom: 12 }} wrap>
        <Input
          placeholder="book_id"
          value={previewBook}
          style={{ width: 240 }}
          onChange={(e) => setPreviewBook(e.target.value)}
        />
        <Input
          placeholder="chapter_id"
          value={previewChapter}
          style={{ width: 240 }}
          onChange={(e) => setPreviewChapter(e.target.value)}
        />
        <Button
          loading={busy}
          onClick={() => {
            setBusy(true);
            previewRules(previewBook, previewChapter)
              .then(setPreview)
              .catch((e) => message.error((e as Error).message))
              .finally(() => setBusy(false));
          }}
        >
          预览
        </Button>
      </Space>
      {preview ? (
        <div>
          <Typography.Paragraph>
            original：
            {preview.original === null ? (
              <Tag color="orange">null（接管）</Tag>
            ) : (
              <pre style={{ whiteSpace: "pre-wrap" }}>{preview.original.slice(0, 500)}</pre>
            )}
          </Typography.Paragraph>
          <Typography.Paragraph>result：</Typography.Paragraph>
          <pre style={{ whiteSpace: "pre-wrap", maxHeight: 240, overflow: "auto" }}>{preview.result.slice(0, 800)}</pre>
          <Typography.Paragraph>
            applied：
            {preview.applied.map((a) => (
              <Tag key={a.rule}>
                {a.rule}：{a.hits} 次
              </Tag>
            ))}
          </Typography.Paragraph>
        </div>
      ) : null}
    </Card>
  );

  return (
    <div>
      <Typography.Title level={4}>内容规则</Typography.Title>
      {globalEditor}
      {bookEditor}
      {previewPanel}
    </div>
  );
}
