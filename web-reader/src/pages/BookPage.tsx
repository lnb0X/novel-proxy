import { useState, type ReactNode } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getBookDetail, getCatalog, type Catalog } from "../api";
import { ErrorRetry } from "../components/ErrorRetry";
import { Skeleton } from "../components/Skeleton";

const PAGE_SIZE = 200;

// 并行请求 detail 与 catalog（DECISIONS：详情页并行请求）。
export function BookPage() {
  const { bookId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const toc = Math.max(1, Number(params.get("toc") ?? "1") || 1);
  const rev = params.get("rev") === "1";
  // 倒序切换：回到目录第 1 页，保留当前章高亮。
  const toggleRev = () => {
    const next = new URLSearchParams(params);
    if (rev) {
      next.delete("rev");
    } else {
      next.set("rev", "1");
    }
    next.set("toc", "1");
    setParams(next);
  };

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
        <div className="toc-head">
          <h2 className="page-title">目录（{cat.total} 章）</h2>
          <button className="toc-sort" type="button" onClick={toggleRev}>
            {rev ? "正序" : "倒序"}
          </button>
        </div>
        {cat.chapters.length === 0 ? (
          <p className="muted">暂无目录</p>
        ) : (
          <TocList
            cat={cat}
            toc={toc}
            totalPages={totalPages}
            bookId={bookId}
            chapterId={chapterId}
            rev={rev}
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

// TocList：每页 200 章；多列网格（参考起点目录），卷头跨全部列；
// 倒序（?rev=1）时序号仍显示真实章节号；当前章高亮。
function TocList(props: {
  cat: Catalog;
  toc: number;
  totalPages: number;
  bookId: string;
  chapterId: string;
  rev: boolean;
}) {
  const { cat, toc, totalPages, bookId, chapterId, rev } = props;
  const page = Math.min(toc, totalPages);
  const start = (page - 1) * PAGE_SIZE;
  const list = rev ? [...cat.chapters].reverse() : cat.chapters;
  const rows = list.slice(start, start + PAGE_SIZE);

  // 章节号按正序全书编号，倒序展示时仍然显示真实序号。
  const ordinals = new Map(cat.chapters.map((c, i) => [c.chapter_id, i + 1]));

  const seenVolumes = new Set<string>();
  const cells: ReactNode[] = [];
  rows.forEach((ch) => {
    const first = !seenVolumes.has(ch.volume);
    seenVolumes.add(ch.volume);
    if (first && ch.volume) {
      cells.push(
        <li key={"v" + ch.chapter_id} className="volume-row">
          <p className="muted volume-head">
            {ch.volume}
            {page > 1 ? "（续）" : ""}
          </p>
        </li>,
      );
    }
    cells.push(
      <li key={ch.chapter_id} className={ch.chapter_id === chapterId ? "current" : ""}>
        <span className="toc-num">{ordinals.get(ch.chapter_id)}</span>
        <Link to={readUrl(bookId, ch.chapter_id) + "?from=toc&toc=" + page}>
          {ch.title}
        </Link>
      </li>,
    );
  });

  const jump = (n: number) =>
    "/books/" + bookId + "?toc=" + n + (chapterId ? "&chapter=" + chapterId : "") + (rev ? "&rev=1" : "");

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
      <ul className="toc">{cells}</ul>
      {pager}
    </div>
  );
}
