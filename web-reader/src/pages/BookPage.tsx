import { Link } from "react-router-dom";
import { SettingsBar, useReaderSettings } from "../components/ReaderSettings";

// /books/:bookId 详情 + 目录（R3 实现完整功能；R1 提供骨架与互相跳转）。
export function BookPage() {
  const settings = useReaderSettings();
  return (
    <div className="page">
      <nav className="topnav">
        <Link to="/">搜索</Link>
        <Link to="/books/7526452300462705689">书详情</Link>
        <Link to="/books/7526452300462705689/read/7526766660229808665">正文</Link>
      </nav>
      <h1 className="page-title">书详情 + 目录</h1>
      <p className="muted">详情与目录在 R3 实现。</p>
      <SettingsBar
        settings={settings.settings}
        setFontSize={settings.setFontSize}
        setTheme={settings.setTheme}
        setFont={settings.setFont}
      />
      <hr className="hr" />
      <div className="prose">
        <p>示例段落，验证排版变量在本页生效。</p>
      </div>
    </div>
  );
}
