import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getBookDetail, getCatalog, getChapterContent } from "../api";
import { ErrorRetry } from "../components/ErrorRetry";
import { Skeleton } from "../components/Skeleton";
import { SettingsBar, useReaderSettings } from "../components/ReaderSettings";

// /books/:bookId/read/:chapterId 正文页。
// 并行请求 content 与 catalog（后者算上一章 / 下一章）。
export function ReaderPage() {
  const { bookId = "", chapterId = "" } = useParams();
  const settings = useReaderSettings();
  const queryClient = useQueryClient();

  const content = useQuery({
    queryKey: ["content", bookId, chapterId],
    queryFn: () => getChapterContent(bookId, chapterId),
  });
  const catalog = useQuery({
    queryKey: ["catalog", bookId],
    queryFn: () => getCatalog(bookId),
  });

  // 预取目录进缓存，便于目录页秒开；书名用于 document.title。
  useEffect(() => {
    queryClient.prefetchQuery({
      queryKey: ["catalog", bookId],
      queryFn: () => getCatalog(bookId),
    });
    queryClient.prefetchQuery({
      queryKey: ["detail", bookId],
      queryFn: () => getBookDetail(bookId),
    });
  }, [bookId, queryClient]);
  const bookDetail = useQuery({
    queryKey: ["detail", bookId],
    queryFn: () => getBookDetail(bookId),
  });

  const cat = catalog.data;
  const idx = cat ? cat.chapters.findIndex((c) => c.chapter_id === chapterId) : -1;
  const prev = cat && idx > 0 ? cat.chapters[idx - 1] : null;
  const next = cat && idx >= 0 && idx < cat.chapters.length - 1 ? cat.chapters[idx + 1] : null;
  // 键盘：左右翻章、空格下翻一屏（桌面端）。
  const navRef = useRef({ prev, next, chapterId, bookId });
  navRef.current = { prev, next, chapterId, bookId };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const n = navRef.current;
      if (e.key === "ArrowLeft" && n.prev) {
        window.location.href = "/books/" + n.bookId + "/read/" + n.prev.chapter_id;
      } else if (e.key === "ArrowRight" && n.next) {
        window.location.href = "/books/" + n.bookId + "/read/" + n.next.chapter_id;
      } else if (e.key === " ") {
        e.preventDefault();
        window.scrollBy({ top: window.innerHeight * 0.85 });
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  // 章节切换：滚回顶部并强制显示菜单。
  useEffect(() => {
    window.scrollTo(0, 0);
    setMenuVisible(true);
  }, [chapterId]);

  // document.title 同步为「章节名 - 书名」。
  const contentData = content.data;
  useEffect(() => {
    if (contentData && cat && idx >= 0) {
      const bookName = bookDetail.data ? bookDetail.data.title : "";
      document.title = bookName ? contentData.title + " - " + bookName : contentData.title;
    }
  }, [contentData, cat, idx]);

  // 菜单显隐状态机（自动隐藏 + 点击正文切换并存）。
  const [menuVisible, setMenuVisible] = useState(true);
  useMenuAutoHide(setMenuVisible);

  // 图片 onerror 隐藏：事件委托。
  useEffect(() => {
    const onImgError = (e: Event) => {
      const t = e.target as HTMLElement;
      if (t.tagName === "IMG") t.classList.add("broken");
    };
    document.addEventListener("error", onImgError, true);
    return () => document.removeEventListener("error", onImgError, true);
  }, []);

  return (
    <div className="page reader-body" onClick={toggleOnBodyClick(setMenuVisible)}>
      <div className="page-container">
        <header className={"topbar" + (menuVisible ? "" : " hidden")}>
        <nav className="topbar-inner">
          <Link className="btn" to={"/books/" + bookId + (idx >= 0 ? "?toc=" + (Math.floor(idx / 200) + 1) + "&chapter=" + chapterId : "")}>
            返回
          </Link>
          <span className="topbar-title">
            {contentData ? contentData.title : ""}
          </span>
          <span className="topbar-spacer" />
        </nav>
      </header>

      <div className="prose" data-chapter={chapterId}>
        {content.isPending ? (
          <Skeleton rows={10} title />
        ) : content.isError ? (
          <ErrorRetry message="正文加载失败" onRetry={() => content.refetch()} />
        ) : (
          <ChapterHtml html={content.data.content} />
        )}
        {content.isSuccess ? (
          <ChapterEnd
            wordCount={plainTextLength(content.data.content)}
            nextChapterId={next ? next.chapter_id : ""}
            bookId={bookId}
            nextTitle={next ? next.title : ""}
          />
        ) : null}
      </div>

      <footer className={"bottombar" + (menuVisible ? "" : " hidden")}>
        <nav className="bottombar-inner">
          {prev ? (
            <Link className="btn" to={"/books/" + bookId + "/read/" + prev.chapter_id}>
              上一章
            </Link>
          ) : (
            <span className="btn disabled">上一章</span>
          )}
          <SettingsPanel settings={settings.settings} setFontSize={settings.setFontSize} setTheme={settings.setTheme} setFont={settings.setFont} />
          {next ? (
            <Link className="btn" to={"/books/" + bookId + "/read/" + next.chapter_id}>
              下一章
            </Link>
          ) : (
            <span className="btn disabled">下一章</span>
          )}
        </nav>
      </footer>
      </div>
    </div>
  );
}

function toggleOnBodyClick(setVisible: React.Dispatch<React.SetStateAction<boolean>>) {
  return (e: React.MouseEvent) => {
    const t = e.target as HTMLElement;
    // 点击顶栏底栏或设置面板不切换。
    if (t.closest(".topbar") || t.closest(".bottombar")) return;
    setVisible((v) => !v);
  };
}

// useMenuAutoHide 滚动自动隐藏：passive + rAF 节流。
// 规则：scrollY <= 8 或距底 <= 8 强制显示；向下滚超 8px 隐藏；
// 向上滚超 8px 显示。点击正文直接翻转状态，与此并存。
function useMenuAutoHide(setVisible: (v: boolean) => void) {
  const lastY = useRef(0);
  const ticking = useRef(false);

  useEffect(() => {
    const onScroll = () => {
      if (ticking.current) return;
      ticking.current = true;
      requestAnimationFrame(() => {
        ticking.current = false;
        const y = window.scrollY;
        const distToBottom =
          document.documentElement.scrollHeight - window.innerHeight - y;
        if (y <= 8 || distToBottom <= 8) {
          setVisible(true);
        } else if (y > lastY.current + 8) {
          setVisible(false);
        } else if (y < lastY.current - 8) {
          setVisible(true);
        }
        lastY.current = y;
      });
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, [setVisible]);
}

// ChapterHtml 渲染后端白名单过滤后的 HTML。
function ChapterHtml(props: { html: string }) {
  return <div dangerouslySetInnerHTML={{ __html: props.html }} />;
}

// ChapterEnd 章末：字数（去标签统计）+ 本章完 + 下一章。
function ChapterEnd(props: {
  wordCount: number;
  nextChapterId: string;
  bookId: string;
  nextTitle: string;
}) {
  return (
    <div className="chapter-end">
      <hr className="hr" />
      <p className="muted">本章字数：{props.wordCount}</p>
      <p className="muted chapter-end-mark">本章完</p>
      {props.nextChapterId ? (
        <Link className="btn" to={"/books/" + props.bookId + "/read/" + props.nextChapterId}>
          下一章：{props.nextTitle}
        </Link>
      ) : (
        <p className="muted">已是最新章节</p>
      )}
    </div>
  );
}

function plainTextLength(html: string): number {
  const div = document.createElement("div");
  div.innerHTML = html;
  return (div.textContent || "").replace(/\s/g, "").length;
}

// SettingsPanel 底栏里的设置面板：字号五档、三主题、两种字体。
function SettingsPanel(props: {
  settings: ReturnType<typeof useReaderSettings>["settings"];
  setFontSize: (n: 16 | 20 | 24 | 28 | 32) => void;
  setTheme: (t: "day" | "sepia" | "night") => void;
  setFont: (f: "sans" | "serif") => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="settings-panel-wrap">
      <button className="btn" onClick={() => setOpen(!open)}>
        字号
      </button>
      {open ? (
        <div className="settings-panel">
          <SettingsBar
            settings={props.settings}
            setFontSize={props.setFontSize}
            setTheme={props.setTheme}
            setFont={props.setFont}
          />
        </div>
      ) : null}
    </div>
  );
}
