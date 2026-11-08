import { expect, test } from "@playwright/test";

// R4 断言 7（单独 test）：正文很短的章节，顶栏底栏全程可见。
// 前置：内容不足一屏（scrollHeight <= innerHeight），菜单本就不该隐藏。
// 使用 7542602438201576472 的第 3 章（3555 字节 JSON，约 1100 汉字），
// 配 1280x2160 大视口确保内容不满屏。
test("短章节顶栏底栏全程可见", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 2160 });
  await page.goto("/books/7542602438201576472/read/7672698526240885784");
  await page.waitForSelector(".prose", { timeout: 20000 });
  await page.waitForFunction(function () { var p = document.querySelector(".prose"); return p && p.innerText.length > 0; });
  await page.waitForTimeout(500);

  // 前置：内容不足一屏（滚不动）。
  const sh = await page.evaluate("document.documentElement.scrollHeight");
  const ih = await page.evaluate("window.innerHeight");
  expect(sh).toBeLessThanOrEqual(ih);

  const topbar = page.locator(".topbar");
  const bottombar = page.locator(".bottombar");

  // 初始可见。
  await expect(topbar).not.toHaveClass(/hidden/);
  await expect(bottombar).not.toHaveClass(/hidden/);

  // 尝试滚动（scrollTo 到超出内容的位置也滚不动）。
  await page.evaluate("window.scrollTo(0, 99999)");
  await page.waitForTimeout(300);

  // 顶栏底栏仍然可见。
  await expect(topbar).not.toHaveClass(/hidden/);
  await expect(bottombar).not.toHaveClass(/hidden/);
});
