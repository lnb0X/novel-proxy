import { createRoot } from "react-dom/client";
import { ReaderApp } from "./ReaderApp";
import { applySettings, loadSettings } from "./settings";

// 首帧前应用阅读设置，避免主题闪烁。
applySettings(loadSettings());

createRoot(document.getElementById("root")!).render(<ReaderApp />);
