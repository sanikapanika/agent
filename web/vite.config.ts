import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

// Production: `vite build` writes dist/, which Go embeds and serves.
// Development (`make run`): Vite runs internally on 127.0.0.1:5173 and the Go
// agent proxies the UI to it, so you always open the agent's own URL
// (http://localhost:8080). Hot reload's websocket goes through that proxy too.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
