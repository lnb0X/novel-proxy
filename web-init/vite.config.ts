import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// base '/init/'：产物挂在 /init/ 下伺服。
export default defineConfig({
  base: "/init/",
  plugins: [react()],
});
