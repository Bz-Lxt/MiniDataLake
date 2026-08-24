import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  timeout: 60_000,
  use: { baseURL: process.env.E2E_BASE || "http://127.0.0.1:18420" },
});
