import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 阅读端挂在站点根路径伺服（frontends app=reader）。
export default defineConfig({
  base: "/",
  plugins: [react()],
});
