import { copyFile, lstat, mkdir, readdir, rm } from "node:fs/promises";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const source = resolve(root, "web/dist");
const target = resolve(root, "internal/webui/dist");

async function checkDirectory(directory) {
  const suffix = relative(root, directory);
  if (!suffix || suffix.startsWith(`..${sep}`)) {
    throw new Error("构建目录必须位于项目内部");
  }
  let current = root;
  for (const part of suffix.split(sep)) {
    current = join(current, part);
    try {
      const info = await lstat(current);
      if (!info.isDirectory() || info.isSymbolicLink()) {
        throw new Error(`构建路径不是普通目录：${current}`);
      }
    } catch (error) {
      if (error.code === "ENOENT") return;
      throw error;
    }
  }
}

async function collect(directory, prefix = "") {
  const files = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const name = join(prefix, entry.name);
    // 内嵌范围只允许构建产物，拒绝配置、源码映射和符号链接。
    if (
      entry.name.startsWith(".") ||
      entry.name.endsWith(".map") ||
      ["src", "source", "node_modules"].includes(entry.name.toLowerCase()) ||
      entry.isSymbolicLink()
    ) {
      throw new Error(`前端构建目录含不可发布内容：${name}`);
    }
    if (entry.isDirectory())
      files.push(...(await collect(join(directory, entry.name), name)));
    else if (entry.isFile()) files.push(name);
    else throw new Error(`前端构建目录含非普通文件：${name}`);
  }
  return files;
}

await checkDirectory(source);
await checkDirectory(target);
const files = await collect(source);
if (!files.includes("index.html"))
  throw new Error("请先构建 web/dist/index.html");

// 清理前核对固定输出路径，只替换项目内的生成目录。
if (relative(root, target) !== join("internal", "webui", "dist")) {
  throw new Error("前端输出目录不符合约定");
}
await rm(target, { recursive: true, force: true });
await mkdir(target, { recursive: true });
for (const name of files) {
  await mkdir(dirname(join(target, name)), { recursive: true });
  await copyFile(join(source, name), join(target, name));
}
console.log(`已内嵌 ${files.length} 个前端文件`);
