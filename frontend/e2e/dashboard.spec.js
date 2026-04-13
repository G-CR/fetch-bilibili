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
          mode: "api",
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
