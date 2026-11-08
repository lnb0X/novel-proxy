// 阅读设置封装：字号 / 主题 / 字体，键名前缀 reader.，存 localStorage。

export type Theme = "day" | "sepia" | "night";
export type Font = "sans" | "serif";

export const FONT_SIZES = [16, 20, 24, 28, 32] as const;
export type FontSize = (typeof FONT_SIZES)[number];

const KEYS = {
  fontSize: "reader.fontSize",
  theme: "reader.theme",
  font: "reader.font",
} as const;

function read<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  const v = localStorage.getItem(key);
  if (v !== null && (allowed as readonly string[]).includes(v)) {
    return v as T;
  }
  return fallback;
}

export interface Settings {
  fontSize: FontSize;
  theme: Theme;
  font: Font;
}

const THEMES: readonly Theme[] = ["day", "sepia", "night"];
const FONTS: readonly Font[] = ["sans", "serif"];

function readSize(key: string): FontSize {
  const v = Number(localStorage.getItem(key));
  return (FONT_SIZES as readonly number[]).includes(v) ? (v as FontSize) : 16;
}

export function loadSettings(): Settings {
  return {
    fontSize: readSize(KEYS.fontSize),
    theme: read(KEYS.theme, THEMES, "day"),
    font: read(KEYS.font, FONTS, "sans"),
  };
}

export function saveFontSize(n: FontSize): void {
  localStorage.setItem(KEYS.fontSize, String(n));
}

export function saveTheme(t: Theme): void {
  localStorage.setItem(KEYS.theme, t);
}

export function saveFont(f: Font): void {
  localStorage.setItem(KEYS.font, f);
}

// applySettings 把三项设置落到 <html> 的 data 属性与 CSS 变量上。
export function applySettings(s: Settings): void {
  const root = document.documentElement;
  root.setAttribute("data-theme", s.theme);
  root.setAttribute("data-font", s.font);
  root.style.setProperty("--reader-font-size", s.fontSize + "px");
}
