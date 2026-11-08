// 阅读设置：三项存 localStorage（reader. 前缀），应用到 <html>。
// R1 提供基础切换控件；R4 正文页以底栏面板形式复用同一状态源。
import { FONT_SIZES, loadSettings, saveFontSize, saveFont, saveTheme, type Font, type FontSize, type Settings, type Theme } from "../settings";
import { useEffect, useState } from "react";

export function useReaderSettings(): {
  settings: Settings;
  setFontSize: (n: FontSize) => void;
  setTheme: (t: Theme) => void;
  setFont: (f: Font) => void;
} {
  const [settings, setSettings] = useState<Settings>(loadSettings);

  useEffect(() => {
    // 初始应用一次，保证直接打开任何路由都带主题。
    apply();
  }, []);

  function apply() {
    // 延迟 import 避免循环依赖（settings.ts 不依赖 React）。
    const root = document.documentElement;
    root.setAttribute("data-theme", settings.theme);
    root.setAttribute("data-font", settings.font);
    root.style.setProperty("--reader-font-size", settings.fontSize + "px");
  }

  function setFontSize(n: FontSize) {
    if (!FONT_SIZES.includes(n)) return;
    saveFontSize(n);
    setSettings((s) => ({ ...s, fontSize: n }));
  }
  function setTheme(t: Theme) {
    saveTheme(t);
    setSettings((s) => ({ ...s, theme: t }));
  }
  function setFont(f: Font) {
    saveFont(f);
    setSettings((s) => ({ ...s, font: f }));
  }

  // 设置变化时重新应用。
  useEffect(apply, [settings]);

  return { settings, setFontSize, setTheme, setFont };
}

// SettingsBar 极简切换控件（三主题 / 字号五档 / 两种字体）。
export function SettingsBar(props: {
  settings: Settings;
  setFontSize: (n: FontSize) => void;
  setTheme: (t: Theme) => void;
  setFont: (f: Font) => void;
}) {
  const { settings } = props;
  return (
    <div className="settings-bar">
      <div className="settings-row">
        <span className="muted">主题</span>
        {(["day", "sepia", "night"] as const).map((t) => (
          <button
            key={t}
            className={"btn" + (settings.theme === t ? " active" : "")}
            onClick={() => props.setTheme(t)}
          >
            {t === "day" ? "日间" : t === "sepia" ? "羊皮" : "夜间"}
          </button>
        ))}
      </div>
      <div className="settings-row">
        <span className="muted">字号</span>
        {FONT_SIZES.map((n) => (
          <button
            key={n}
            className={"btn" + (settings.fontSize === n ? " active" : "")}
            onClick={() => props.setFontSize(n)}
          >
            {n}
          </button>
        ))}
      </div>
      <div className="settings-row">
        <span className="muted">字体</span>
        {(["sans", "serif"] as const).map((f) => (
          <button
            key={f}
            className={"btn" + (settings.font === f ? " active" : "")}
            onClick={() => props.setFont(f)}
          >
            {f === "sans" ? "无衬线" : "衬线"}
          </button>
        ))}
      </div>
    </div>
  );
}
