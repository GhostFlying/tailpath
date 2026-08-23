import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, ".", "");
  const apiTarget = environment.TAILPATH_API_URL ?? "http://127.0.0.1:8080";
  return {
    plugins: [react()],
    server: {
      proxy: {
        "/api": apiTarget,
        "/healthz": apiTarget,
      },
    },
  };
});
