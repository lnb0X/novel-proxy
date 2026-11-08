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
  // 失败时输出页面文本便于排查（上游偶发 50201/50203）。
  page.on("requestfailed", function (r) { console.log("REQ FAIL:", r.url()); });
  await page.getByRole("link", { name: "开始阅读" }).or(page.getByText("目录加载失败")).waitFor({ timeout: 30000 });
  // 开始阅读 → 正文页。
  await page.getByRole("link", { name: "开始阅读" }).click();
  await page.waitForURL(/\/read\/\d+/, { timeout: 20000 });
  await page.waitForSelector(".prose p", { timeout: 20000 });
  // 正文有实际内容且落在页面上。
  const text = await page.locator(".prose").innerText();
  expect(text.length).toBeGreaterThan(100);
  // 标题同步为「章节名 - 书名」。
  await expect(page).toHaveTitle(/.+ - .+/);
});
