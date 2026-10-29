import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // The Go service serves the built frontend from ./static (see
    // src/api/main.go). The azd prebuild hook runs this build before the
    // container image is built.
    outDir: "../api/static",
    emptyOutDir: true,
    target: "esnext",
  },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
