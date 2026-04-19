import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const versionFile = path.join(repoRoot, "internal", "version", "VERSION");
const hostforgeVersion = fs
  .readFileSync(versionFile, "utf8")
  .trim()
  .split(/\r?\n/)[0]!
  .trim()
  .replace(/^v/, "");

export default defineConfig({
  define: {
    __HOSTFORGE_VERSION__: JSON.stringify(hostforgeVersion),
  },
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
        ws: true,
      },
      "/hooks": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/auth": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
