import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  // Tauri 开发时由 Rust 侧提供服务；纯前端 mock 模式走默认端口
  clearScreen: false,
  server: { port: 5173, strictPort: true },
});
