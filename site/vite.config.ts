import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  ssgOptions: {
    entry: "src/main.tsx",
    dirStyle: "nested",
  },
});
