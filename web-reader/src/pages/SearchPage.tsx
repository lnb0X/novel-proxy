import { Link } from "react-router-dom";
import { SettingsBar, useReaderSettings } from "../components/ReaderSettings";
import { applySettings } from "../settings";

// / 搜索页（R2 实现完整功能；R1 提供骨架与互相跳转）。
export function SearchPage() {
  const settings = useReaderSettings();
  applySettings(settings.settings);
  return (
    <div className="page">
      <nav className="topnav">
        <Link to="/">搜索</Link>
        <Link to="/books/7526452300462705689">书详情</Link>
        <Link to="/books/7526452300462705689/read/7526766660229808665">正文</Link>
      </nav>
      <h1 className="page-title">搜索</h1>
      <p className="muted">搜索功能在 R2 实现。</p>
      <SettingsBar
        settings={settings.settings}
        setFontSize={settings.setFontSize}
        setTheme={settings.setTheme}
        setFont={settings.setFont}
      />
      <hr className="hr" />
      <div className="prose">
        <p>
          示例正文段落，用于验证排版变量：字号、行高 1.85、列宽 38em、段间距 0.8em。
          主题与字体的切换由上方控件完成，全部来自 reader.css 的 CSS 变量层。
        </p>
      </div>
    </div>
  );
}
