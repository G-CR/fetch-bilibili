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
  if (!html.includes("系统概况")) {
    throw new Error("前端未渲染系统概况标题");
  }
  if (!html.includes("实时连接状态")) {
    throw new Error("前端未渲染实时连接状态");
  }
  if (!html.includes("博主管理与追踪状态")) {
    throw new Error("前端未渲染博主管理标题");
  }
  if (!html.includes("任务详情")) {
    throw new Error("前端未渲染任务详情面板");
  }
  if (!html.includes("清理候选预览")) {
    throw new Error("前端未渲染清理预览面板");
  }
  if (!html.includes("选择任务查看详情")) {
    throw new Error("前端未渲染任务详情空态文案");
  }
  if (!html.includes("添加博主")) {
    throw new Error("前端未渲染博主管理表单");
  }
  if (!html.includes("本地视频")) {
    throw new Error("前端未渲染博主本地视频列");
  }
  if (!html.includes("占用空间")) {
    throw new Error("前端未渲染博主占用空间列");
  }
  if (!html.includes("前端连接设置")) {
    throw new Error("前端未渲染连接设置面板");
  }
  if (!html.includes("配置文件编辑")) {
    throw new Error("前端未渲染配置编辑面板");
  }
  if (!html.includes("候选池与人工审核")) {
    throw new Error("前端未渲染候选池页面标题");
  }
  if (!html.includes("每页显示")) {
    throw new Error("前端未渲染分页配置控件");
  }
  if (!html.includes("上一页") || !html.includes("下一页")) {
    throw new Error("前端未渲染分页切换按钮");
  }
  if (!html.includes("手动发现")) {
    throw new Error("前端未渲染候选池发现按钮");
  }
  if (!html.includes("选择候选查看来源与评分拆解")) {
    throw new Error("前端未渲染候选池详情抽屉空态");
  }
  if (!html.includes("保存前差异预览")) {
    throw new Error("前端未渲染配置差异预览面板");
  }
  if (!html.includes("校验结果详情")) {
    throw new Error("前端未渲染配置校验详情面板");
  }
  if (html.includes("第一屏直接展示实时工作态")) {
    throw new Error("前端仍保留宣传式系统概况标题");
  }
  if (html.includes("维护入口与状态都集中在这里")) {
    throw new Error("前端仍保留宣传式博主管理标题");
  }
  if (html.includes("总览驾驶舱")) {
    throw new Error("前端仍保留驾驶舱文案");
  }
  if (html.includes("本地模式")) {
    throw new Error("前端仍保留本地模式文案");
  }
  if (html.includes('name="mode"')) {
    throw new Error("前端仍保留模式切换控件");
  }

  console.log("smoke render ok");
} finally {
  await rm(tempDir, { recursive: true, force: true });
}
