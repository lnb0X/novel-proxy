import { Link } from "react-router-dom";
import { SettingsBar, useReaderSettings } from "../components/ReaderSettings";

// /books/:bookId/read/:chapterId 正文页（R4 实现完整功能）。
export function ReaderPage() {
  const settings = useReaderSettings();
  return (
    <div className="page reader-body">
      <nav className="topnav">
        <Link to="/">搜索</Link>
        <Link to="/books/7526452300462705689">书详情</Link>
        <Link to="/books/7526452300462705689/read/7526766660229808665">正文</Link>
      </nav>
      <h1 className="page-title">正文</h1>
      <p className="muted">正文功能在 R4 实现。</p>
      <SettingsBar
        settings={settings.settings}
        setFontSize={settings.setFontSize}
        setTheme={settings.setTheme}
        setFont={settings.setFont}
      />
      <hr className="hr" />
      <div className="prose">
        <p>正文排版示例段落，列宽 38em 随字号变化。</p>
      </div>
    </div>
  );
}
