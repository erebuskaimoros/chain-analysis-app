import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/web/ui/dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
  },
  test: {
    environment: "jsdom",
  },
});
