import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { searchBooks, type SearchResult } from "../api";
import { ErrorRetry } from "../components/ErrorRetry";
import { Skeleton } from "../components/Skeleton";

// 输入框草稿与 URL 的 q 分离：回车才提交。
export function SearchPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const page = Math.max(1, Number(params.get("page") ?? "1") || 1);
  const [draft, setDraft] = useState(q);

  useEffect(() => {
    setDraft(q);
  }, [q]);

  const submit = () => {
    const t = draft.trim();
    if (t === "") {
      // 空 q 按 40001 处理，同步清 URL。
      setParams({});
      return;
    }
    setParams({ q: t, page: "1" });
  };

  const results = useQuery({
    queryKey: ["search", q, page],
    queryFn: () => searchBooks(q, page),
    enabled: q !== "",
  });

  return (
    <div className="page">
      <nav className="topnav">
        <Link to="/">搜索</Link>
      </nav>
      <h1 className="page-title">搜索</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <input
          className="search-input"
          value={draft}
          placeholder="输入书名或作者，回车搜索"
          onChange={(e) => setDraft(e.target.value)}
        />
        <button className="btn" type="submit">
          搜索
        </button>
      </form>
      <SearchBody q={q} page={page} results={results} goPage={(p) => setParams({ q, page: String(p) })} />
    </div>
  );
}

function SearchBody(props: {
  q: string;
  page: number;
  results: ReturnType<typeof useQuery<SearchResult, Error>>;
  goPage: (p: number) => void;
}) {
  const { q, page, results, goPage } = props;

  // 累积结果：第 1 页覆盖，第 N>1 页追加（「加载更多」语义）。
  // q 变化或回到第 1 页时重置。
  const [items, setItems] = useState<SearchResult["items"]>([]);
  const [meta, setMeta] = useState({ hasMore: false, loadedPage: 0 });
  const lastKey = useRef("");

  useEffect(() => {
    if (!results.data) return;
    const d = results.data;
    if (lastKey.current !== q + "#" + page) {
      lastKey.current = q + "#" + page;
    }
    setItems((prev) => {
      if (page <= 1) return d.items;
      const seen = new Set(prev.map((x) => x.book_id));
      return [...prev, ...d.items.filter((x) => !seen.has(x.book_id))];
    });
    setMeta({ hasMore: d.has_more, loadedPage: page });
  }, [results.data, q, page]);


  // 空 q：提示而非静默。
  if (q === "") {
    return <p className="muted">输入书名或作者开始搜索。</p>;
  }
  if (results.isPending) {
    // 骨架屏：8 行静态色块。
    return (
      <div>
        <Skeleton rows={2} title />
        <Skeleton rows={2} title />
        <Skeleton rows={2} title />
        <Skeleton rows={2} title />
      </div>
    );
  }
  if (results.isError) {
    // 上游失败文案与空结果分开（验收项）。
    return <ErrorRetry message="搜索服务暂时不可用" onRetry={() => results.refetch()} />;
  }
  if (results.isSuccess && results.data.items.length === 0 && page <= 1) {
    // 空结果文案（与上游失败不同）。
    return <p className="muted">没有找到与「{q}」相关的书。</p>;
  }
  const data = { items, has_more: meta.hasMore };

  return (
    <div>
      {data.items.map((it) => (
        <Link className="result-item" key={it.book_id} to={"/books/" + it.book_id}>
          {it.thumb_url ? (
            <img className="cover" src={it.thumb_url} alt="" loading="lazy" />
          ) : (
            <div className="cover skeleton" />
          )}
          <div className="meta">
            <p className="title">{it.title}</p>
            <p className="sub">
              {it.author}
              {it.category ? " · " + it.category : ""}
            </p>
            <p className="abstract">{it.abstract}</p>
          </div>
        </Link>
      ))}
      {data.has_more ? (
        <p style={{ textAlign: "center" }}>
          <button className="btn" onClick={() => goPage(meta.loadedPage + 1)}>
            加载更多（第 {meta.loadedPage + 1} 页）
          </button>
        </p>
      ) : (
        <p className="muted" style={{ textAlign: "center" }}>
          没有更多了
        </p>
      )}
    </div>
  );
}
