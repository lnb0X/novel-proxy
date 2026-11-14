import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getBookDetail, getCatalog, type Catalog } from "../api";
import { ErrorRetry } from "../components/ErrorRetry";
import { Skeleton } from "../components/Skeleton";

const PAGE_SIZE = 200;

// 并行请求 detail 与 catalog（DECISIONS：详情页并行请求）。
export function BookPage() {
  const { bookId = "" } = useParams();
  const [params] = useSearchParams();
  const toc = Math.max(1, Number(params.get("toc") ?? "1") || 1);

  const [introOpen, setIntroOpen] = useState(false);
  const detail = useQuery({
    queryKey: ["detail", bookId],
    queryFn: () => getBookDetail(bookId),
  });
  const catalog = useQuery({
    queryKey: ["catalog", bookId],
    queryFn: () => getCatalog(bookId),
  });

  if (detail.isPending || catalog.isPending) {
    return (
      <div className="page">
        <TopNav />
        <div className="page-container">
          <Skeleton rows={4} title cover />
          <Skeleton rows={8} />
        </div>
      </div>
    );
  }
  if (detail.isError) {
    return (
      <div className="page">
        <TopNav />
        <div className="page-container">
          <ErrorRetry message="书籍信息加载失败" onRetry={() => detail.refetch()} />
        </div>
      </div>
    );
  }
  if (catalog.isError) {
    return (
      <div className="page">
        <TopNav />
        <div className="page-container">
          <ErrorRetry message="目录加载失败" onRetry={() => catalog.refetch()} />
        </div>
      </div>
    );
  }

  const d = detail.data;
  const cat = catalog.data;
  const chapterId = currentChapterId(params, cat);
  const totalPages = Math.max(1, Math.ceil(cat.chapters.length / PAGE_SIZE));

  return (
    <div className="page">
      <TopNav />
      <div className="page-container">
        <div className="book-head">
          {d.thumb_url ? <img className="book-cover" src={d.thumb_url} alt="" /> : null}
          <div className="book-meta">
            <h1 className="page-title">{d.title}</h1>
            <p className="sub">
              {d.author} · {formatWords(d.word_number)}字
            </p>
          </div>
        </div>
        <div>
          <p className={"intro" + (introOpen ? "" : " intro-clamp")}>{d.intro}</p>
          {d.intro.length > 80 ? (
            <button className="intro-toggle" onClick={() => setIntroOpen(!introOpen)}>
              {introOpen ? "收起" : "展开"}
            </button>
          ) : null}
        </div>
        <p>
          <Link className="btn primary" to={readUrl(bookId, firstChapterId(cat))}>
            开始阅读
          </Link>
        </p>
        <hr className="hr" />
        <h2 className="page-title">目录（{cat.total} 章）</h2>
        {cat.chapters.length === 0 ? (
          <p className="muted">暂无目录</p>
        ) : (
          <TocList
            cat={cat}
            toc={toc}
            totalPages={totalPages}
            bookId={bookId}
            chapterId={chapterId}
          />
        )}
      </div>
    </div>
  );
}

// currentChapterId 从 ?chapter= 读当前章（正文页“目录”返回时带上），
// 没有则取第一章。
function currentChapterId(params: URLSearchParams, cat: Catalog): string {
  const c = params.get("chapter");
  if (c) return c;
  return firstChapterId(cat);
}

function firstChapterId(cat: Catalog): string {
  return cat.chapters.length > 0 ? cat.chapters[0].chapter_id : "";
}

function readUrl(bookId: string, chapterId: string): string {
  return "/books/" + bookId + "/read/" + chapterId;
}

function formatWords(n: number): string {
  if (n >= 10000) {
    return (n / 10000).toFixed(1).replace(/\.0$/, "") + " 万";
  }
  return String(n);
}

function TopNav() {
  return (
    <nav className="topnav">
      <Link to="/">搜索</Link>
    </nav>
  );
}

// TocList：每页 200 章；跨页的卷头显示「（续）」；当前章高亮。
function TocList(props: {
  cat: Catalog;
  toc: number;
  totalPages: number;
  bookId: string;
  chapterId: string;
}) {
  const { cat, toc, totalPages, bookId, chapterId } = props;
  const page = Math.min(toc, totalPages);
  const start = (page - 1) * PAGE_SIZE;
  const rows = cat.chapters.slice(start, start + PAGE_SIZE);

  const seenVolumes = new Set<string>();
  const rowsWithVolume = rows.map((ch, i) => {
    const first = !seenVolumes.has(ch.volume);
    seenVolumes.add(ch.volume);
    return { ...ch, firstOfVolume: first, ordinal: start + i + 1 };
  });

  const jump = (n: number) => "/books/" + bookId + "?toc=" + n + (chapterId ? "&chapter=" + chapterId : "");

  const pager = totalPages > 1 ? (
    <p className="toc-pager">
      第 {page} / {totalPages} 页
      {page > 1 ? (
        <Link className="toc-link" to={jump(page - 1)}>
          上一页
        </Link>
      ) : null}
      {page < totalPages ? (
        <Link className="toc-link" to={jump(page + 1)}>
          下一页
        </Link>
      ) : null}
    </p>
  ) : null;

  return (
    <div>
      <ul className="toc">
        {rowsWithVolume.map((ch) => (
          <li key={ch.chapter_id} className={ch.chapter_id === chapterId ? "current" : ""}>
            {ch.firstOfVolume && ch.volume ? (
              <p className="muted volume-head">
                {ch.volume}
                {page > 1 ? "（续）" : ""}
              </p>
            ) : null}
            <span className="toc-num">{ch.ordinal}</span>
            <Link to={readUrl(bookId, ch.chapter_id) + "?from=toc&toc=" + page}>
              {ch.title}
            </Link>
          </li>
        ))}
      </ul>
      {pager}
    </div>
  );
}
