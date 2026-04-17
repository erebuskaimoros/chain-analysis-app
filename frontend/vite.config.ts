import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/web/ui/dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.indexOf("node_modules/cytoscape") >= 0) {
            return "cytoscape-vendor";
          }
          if (id.indexOf("node_modules/elkjs") >= 0) {
            return "elk-vendor";
          }
          if (id.indexOf("node_modules/@tanstack/react-query") >= 0) {
            return "react-query-vendor";
          }
          if (id.indexOf("node_modules/react") >= 0 || id.indexOf("node_modules/react-dom") >= 0) {
            return "react-vendor";
          }
          return undefined;
        },
      },
    },
  },
  server: {
    port: 5173,
  },
  test: {
    environment: "jsdom",
  },
});
