import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
var repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
var versionFile = path.join(repoRoot, "internal", "version", "VERSION");
var hostforgeVersion = fs
    .readFileSync(versionFile, "utf8")
    .trim()
    .split(/\r?\n/)[0]
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
                // Long builds can go quiet for minutes; avoid dev proxy closing the log WebSocket as "idle".
                timeout: 0,
                proxyTimeout: 0,
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
