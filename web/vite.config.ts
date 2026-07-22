import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig(({ mode }) => {
  const demo = mode === "demo";
  return {
    base: demo ? "./" : "/",
    plugins: [react()],
    build: {
      emptyOutDir: true,
      outDir: demo ? "dist" : "../internal/webassets/dist",
      sourcemap: false,
      chunkSizeWarningLimit: 800,
    },
  };
});
