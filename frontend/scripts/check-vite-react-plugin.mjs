import path from "node:path";
import { pathToFileURL } from "node:url";

const configPath = path.resolve("vite.config.js");
const module = await import(pathToFileURL(configPath).href);
const config =
  typeof module.default === "function" ? await module.default({ command: "build", mode: "production" }) : module.default;

const plugins = Array.isArray(config?.plugins) ? config.plugins.flat(Infinity) : [];
const hasReactPlugin = plugins.some((plugin) => {
  const name = typeof plugin?.name === "string" ? plugin.name : "";
  return name.includes("react");
});

if (!hasReactPlugin) {
  throw new Error("vite.config.js 未配置 React 插件");
}

console.log("vite react plugin ok");
