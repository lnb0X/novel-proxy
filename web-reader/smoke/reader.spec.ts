import { expect, test } from "@playwright/test";

// 冒烟一：页面能加载且标题正确。
test("页面能加载且标题正确", async ({ page }) => {
  await page.goto("/books/7526452300462705689/read/7526766660229808665");
  await expect(page.locator(".prose")).toBeVisible({ timeout: 15000 });
  // document.title 同步为「章节名 - 书名」。
  await expect(page).toHaveTitle(/.+ - .+/);
});

// 冒烟二：正文图片 src 不以 data: 开头（后端剥图规则：data: 占位不外发）。
test("正文图片 src 不以 data: 开头", async ({ page }) => {
  // 7173216089122439711 是带图章节。
  await page.goto("/books/7526452300462705689/read/7173216089122439711");
  await page.waitForSelector(".prose", { timeout: 15000 });
  await page.waitForTimeout(1000); // 图片请求
  const imgs = await page.locator(".prose img").all();
  for (const img of imgs) {
    const src = await img.getAttribute("src");
    if (src) {
      expect(src.startsWith("data:")).toBe(false);
    }
  }
});

// 冒烟三：点下一章后 URL 里的 chapterId 变化。
test("点下一章后 URL 里的 chapterId 变化", async ({ page }) => {
  await page.goto("/books/7526452300462705689/read/7526766660229808665");
  await page.waitForSelector(".prose", { timeout: 15000 });
  const before = page.url();
  await page.locator(".bottombar-inner a", { hasText: "下一章" }).click();
  await page.waitForTimeout(1500);
  const after = page.url();
  expect(after).not.toBe(before);
  expect(after).toMatch(/\/read\/\d+$/);
});

// 冒烟四（R6 验收）：从搜索到读完一章的完整链路。
// 搜索结果的首条 book_id 随上游变化（可能命中无目录的书），
// 所以进详情页用固定 book_id（7526452300462705689，107 章）。
test("从零部署后能完整读一章", async ({ page }) => {
  // 搜索。
  await page.goto("/");
  await page.getByPlaceholder("输入书名或作者，回车搜索").fill("斗破苍穹");
  await page.getByRole("button", { name: "搜索" }).click();
  await page.waitForSelector(".result-item", { timeout: 20000 });
  // 进详情页（固定 book_id，有章节）。
  await page.goto("/books/7526452300462705689");
  // 等目录加载出「开始阅读」或「目录加载失败」（上游偶发 50201/50203）。
  await page
    .getByRole("link", { name: "开始阅读" })
    .or(page.getByText("目录加载失败"))
    .waitFor({ timeout: 30000 });
  // 开始阅读 → 正文页。
  await page.getByRole("link", { name: "开始阅读" }).click();
  await page.waitForURL(/\/read\/\d+/, { timeout: 20000 });
  await page.waitForSelector(".prose", { timeout: 20000 });
  await page.waitForFunction(function () { var p = document.querySelector(".prose"); return p && p.innerText.length > 0; });
  // 正文有实际内容且落在页面上。
  const text = await page.locator(".prose").innerText();
  expect(text.length).toBeGreaterThan(100);
  // 标题同步为「章节名 - 书名」。
  await expect(page).toHaveTitle(/.+ - .+/);
});

// R4 核心交互：顶栏底栏自动隐藏（断言 1-6）。
// 前置：所选章节内容必须超过一屏（scrollHeight > innerHeight），
// 否则第 2 步根本滚不动，测试会假通过。
// 断言用 evaluate 读 classList（Playwright toHaveClass 的自动重试
// 与 React 状态更新交互不稳定，evaluate 一次性读取更可靠）。
test("顶栏底栏自动隐藏", async ({ page }) => {
  test.setTimeout(30000);
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto("/books/7526452300462705689/read/7173216089122439711");
  await page.waitForSelector(".prose", { timeout: 20000 });
  await page.waitForTimeout(500);
  const sh = await page.evaluate("document.documentElement.scrollHeight");
  expect(sh).toBeGreaterThan(800);

  // 单次 evaluate 内完成「滚动 → 等 rAF → 读 class」，无竞态。
  const step = function (y: number) {
    return page.evaluate(function (t) {
      return new Promise(function (resolve) {
        window.scrollTo(0, t);
        setTimeout(function () {
          resolve(document.querySelector(".topbar").className);
        }, 300);
      });
    }, y);
  };

  // 1. 初始可见。
  const c0 = await step(0);
  expect(c0).not.toContain("hidden");

  // 2. 向下滚 → hidden。
  const c1 = await step(400);
  expect(c1).toContain("hidden");

  // 3. 向上滚 → visible。
  const c2 = await step(200);
  expect(c2).not.toContain("hidden");

  // 4. 到顶 → visible。
  const c3 = await step(0);
  expect(c3).not.toContain("hidden");

  // 5. 到底 → bottombar visible。
  const c4 = await page.evaluate(function () {
    return new Promise(function (resolve) {
      window.scrollTo(0, document.documentElement.scrollHeight);
      setTimeout(function () {
        resolve(document.querySelector(".bottombar").className);
      }, 300);
    });
  });
  expect(c4).not.toContain("hidden");

  // 6. 点正文翻转（双向）。从底部滚回 400 是向上滚，先确认可见；
  // 用视口固定坐标点击，不引发滚动，翻转结果才确定。
  const before = await step(400);
  expect(before).not.toContain("hidden");
  await page.mouse.click(640, 300);
  await page.waitForTimeout(300);
  const after = await page.evaluate("document.querySelector('.topbar').className");
  expect(after).toContain("hidden");
  // 再点一次翻回显示。
  await page.mouse.click(640, 300);
  await page.waitForTimeout(300);
  const back = await page.evaluate("document.querySelector('.topbar').className");
  expect(back).toBe(before);
});
