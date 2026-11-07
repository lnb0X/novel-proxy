import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// base './'：产物挂在任意路径（/<secret>/）下都能加载资源。
export default defineConfig({
  base: "./",
  plugins: [react()],
  server: {
    port: 5173,
  },
});
