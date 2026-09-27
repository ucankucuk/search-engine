import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Vite config'i minimal tutuyoruz — sadece React plugin'i. Proxy tanımlamadık,
// API adresini .env üzerinden (VITE_API_URL) veriyoruz çünkü Docker Compose
// içinde de, lokal geliştirmede de aynı mekanizma çalışsın istiyoruz.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
  },
});
