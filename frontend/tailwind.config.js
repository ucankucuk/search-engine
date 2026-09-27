/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        // Marka renk paleti — birincil/ikincil ve üç aksan rengi.
        brand: {
          primary: "#2DC44D", // Birincil — CTA'lar, başarı/yüksek skor
          secondary: "#0087FF", // İkincil — rozetler, orta skor, vurgular
          navy: "#1b2f6f", // Koyu zemin tonu
          red: "#ff3b42", // Uyarı / düşük skor
          purple: "#9966ff", // Aksan — hero glow, "akıllı arama" hissi
        },
        score: {
          low: "#ff3b42",
          mid: "#0087FF",
          high: "#2DC44D",
        },
      },
      boxShadow: {
        glow: "0 0 40px rgba(0, 135, 255, 0.25)",
        "glow-sm": "0 0 20px rgba(153, 102, 255, 0.2)",
      },
    },
  },
  plugins: [],
};
