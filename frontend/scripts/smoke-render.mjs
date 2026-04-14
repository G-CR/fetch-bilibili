import path from "node:path";
import { mkdtemp, rm } from "node:fs/promises";
import { pathToFileURL } from "node:url";
import React from "react";
import { renderToString } from "react-dom/server";
import { build } from "esbuild";

const tempDir = await mkdtemp(path.join(process.cwd(), ".smoke-render-"));
const outfile = path.join(tempDir, "app.mjs");

try {
  await build({
    entryPoints: [path.resolve("src/App.jsx")],
    outfile,
    bundle: true,
    format: "esm",
    platform: "node",
    jsx: "automatic",
    jsxImportSource: "react",
    external: ["react", "react-dom", "react-dom/server", "react/jsx-runtime"]
  });

  const module = await import(pathToFileURL(outfile).href);
  const App = module.default;
  const html = renderToString(React.createElement(App));

  if (!html.includes("绝版视频库")) {
    throw new Error("前端首屏未渲染出预期文案");
  }
  if (!html.includes("任务详情")) {
    throw new Error("前端未渲染任务详情面板");
  }
  if (!html.includes("清理预览")) {
    throw new Error("前端未渲染清理预览面板");
  }
  if (!html.includes("失败原因")) {
    throw new Error("前端未渲染失败原因区域");
  }
  if (!html.includes("停止追踪")) {
    throw new Error("前端未渲染停止追踪操作文案");
  }

  console.log("smoke render ok");
} finally {
  await rm(tempDir, { recursive: true, force: true });
}
