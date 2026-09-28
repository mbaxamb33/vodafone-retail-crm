import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// API_URL lets the frontend target a backend on a non-default address.
const api = process.env.API_URL ?? "http://127.0.0.1:8080";
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": api,
      "/health": api,
    },
  },
});
