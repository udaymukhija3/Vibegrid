import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        ink: "#17231f",
        paper: "#edf4f1",
        card: "#fffdf7",
        line: "#c9d8d1",
        mint: "#6fd1aa",
        tomato: "#ee6f5e",
        yolk: "#f4c95d",
        plum: "#7764d8",
        pool: "#67bdd0"
      },
      boxShadow: {
        soft: "0 18px 42px rgba(23, 35, 31, 0.08)",
        lift: "0 22px 52px rgba(23, 35, 31, 0.13)",
        tile: "0 9px 20px rgba(23, 35, 31, 0.09)"
      }
    }
  },
  plugins: []
};

export default config;
