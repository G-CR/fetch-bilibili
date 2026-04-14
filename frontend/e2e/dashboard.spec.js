import { expect, test } from "@playwright/test";

const STORAGE_KEY = "bili-vault-dashboard-v3";
const apiBase =
  process.env.E2E_API_BASE || (process.env.E2E_MODE === "live" ? "http://127.0.0.1:8080" : "http://127.0.0.1:43180");
const isLive = process.env.E2E_MODE === "live";

test.beforeEach(async ({ page, request }) => {
  if (!isLive) {
    await resetMockState(request);
  }

  await page.addInitScript(
    ({ storageKey, nextApiBase }) => {
      window.localStorage.setItem(
        storageKey,
        JSON.stringify({
          apiBase: nextApiBase
        })
      );
    },
    { storageKey: STORAGE_KEY, nextApiBase: apiBase }
  );
});

test("页面可以打开并同步接口数据", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByTestId("sync-button")).toBeVisible();
  await expect(page.getByText("本地模式", { exact: true })).toHaveCount(0);
  await page.getByTestId("sync-button").click();

  if (!isLive) {
    await expect(page.getByText("Mock 收藏向频道")).toBeVisible();
    await expect(page.getByTestId("job-detail-panel").getByText("Mock 视频接口返回 412")).toBeVisible();
  }
});

test("可以通过 API 模式添加博主", async ({ page }) => {
  const creatorUID = String(Date.now());
  const creatorName = `E2E 博主 ${creatorUID}`;

  await page.goto("/");
  await page.getByTestId("sync-button").click();

  await page.getByLabel("UID").fill(creatorUID);
  await page.getByLabel("名称").fill(creatorName);
  await page.getByTestId("creator-submit").click();

  await expect(page.getByTestId("creator-list").getByText(creatorUID, { exact: true })).toBeVisible();
  await expect(page.getByTestId("creator-list").getByText(creatorName, { exact: true })).toBeVisible();
});

test("可以通过 API 模式暂停和启用博主", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("sync-button").click();

  const creatorRow = page.locator("[data-testid='creator-list'] .table-row").filter({ hasText: "123456" });
  await expect(creatorRow).toContainText("active");
  await creatorRow.getByRole("button", { name: "暂停" }).click();
  await expect(creatorRow).toContainText("paused");
  await expect(creatorRow.getByRole("button", { name: "启用" })).toBeVisible();

  await creatorRow.getByRole("button", { name: "启用" }).click();
  await expect(creatorRow).toContainText("active");
  await expect(creatorRow.getByRole("button", { name: "暂停" })).toBeVisible();
});

test("可以加载并保存系统配置", async ({ page }) => {
  await page.goto("/");

  const editor = page.getByTestId("config-editor");
  await expect(editor).toBeVisible();
  const before = await editor.inputValue();
  expect(before).toContain("server:");

  await editor.fill(`${before}\n# e2e save`);
  await expect(page.getByTestId("config-diff-preview")).toContainText("# e2e save");
  await expect(page.getByText("有未保存修改")).toBeVisible();
  await page.getByTestId("config-save-button").click();

  await expect(page.getByText("配置已保存，后端正在重启")).toBeVisible();
});

test("配置校验失败时会展示错误详情", async ({ page }) => {
  await page.goto("/");

  const editor = page.getByTestId("config-editor");
  await expect(editor).toBeVisible();

  await editor.fill("server:\n\tbad: true");
  await page.getByTestId("config-save-button").click();

  await expect(page.getByTestId("config-validation-detail")).toContainText("配置校验失败");
  await expect(page.getByTestId("config-validation-detail")).toContainText("Tab 缩进");
});

test("可以触发任务并查看任务详情", async ({ page }) => {
  await page.goto("/");
  await page.getByTestId("sync-button").click();
  await page.getByTestId("quick-action-fetch").click();

  await expect(page.getByTestId("job-list")).toContainText("拉取最新视频");
  await page.getByTestId("job-list").getByRole("button").first().click();
  await expect(page.getByTestId("job-detail-panel")).toContainText("任务详情");
  await expect(page.getByTestId("job-detail-panel")).toContainText("Payload");
});

async function resetMockState(request) {
  let lastError = null;

  for (let index = 0; index < 20; index += 1) {
    try {
      await request.post(`${apiBase}/__reset`);
      return;
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 300));
    }
  }

  throw lastError || new Error("mock api reset failed");
}
