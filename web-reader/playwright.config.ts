import { defineConfig } from "@playwright/test";

// 冒烟测试跑在已部署的阅读端上（bin/app.exe -static 或 frontends 默认目录）。
export default defineConfig({
  use: {
    baseURL: "http://127.0.0.1:8080",
  },
  testDir: "./smoke",
  timeout: 30000,
  retries: 0,
  workers: 1,
});
