import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    host: "127.0.0.1",
    port: Number(process.env.PORT) || 43111,
    strictPort: true,
  },
});
