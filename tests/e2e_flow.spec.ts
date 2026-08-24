import { test, expect } from "@playwright/test";

const base = process.env.E2E_BASE || "http://127.0.0.1:18420";

test("workspace shell renders", async ({ page }) => {
  await page.goto(base);
  await expect(page.getByRole("heading", { name: /MINI DATA LAKE/i }).or(page.getByText("MINI DATA LAKE"))).toBeVisible();
  await expect(page.getByText("拖入或选择文件")).toBeVisible();
});

test("upload csv then query", async ({ page }) => {
  await page.goto(base);
  const file = "testdata/samples/users.csv";
  await page.locator("input[type=file]").setInputFiles(file);
  await expect(page.getByText("users")).toBeVisible({ timeout: 30000 });
  await page.getByRole("button", { name: /执行/ }).click();
  await expect(page.getByText(/rows/)).toBeVisible({ timeout: 20000 });
});
