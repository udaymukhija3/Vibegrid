import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        ink: "#171717",
        paper: "#f8fafc",
        mint: "#2ec4b6",
        tomato: "#ff6b6b",
        yolk: "#f9c74f",
        plum: "#6d5dfc"
      },
      boxShadow: {
        tile: "0 12px 0 rgba(23, 23, 23, 0.08)"
      }
    }
  },
  plugins: []
};

export default config;

