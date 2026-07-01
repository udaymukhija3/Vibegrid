import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        ink: "#223027",
        paper: "#f3f7f1",
        card: "#fffdfa",
        line: "#d3ddd3",
        mint: "#77d3b0",
        tomato: "#f06f64",
        yolk: "#f3ca68",
        plum: "#6257b7"
      },
      boxShadow: {
        soft: "0 18px 45px rgba(34, 48, 39, 0.08)",
        lift: "0 22px 55px rgba(34, 48, 39, 0.12)",
        tile: "0 10px 24px rgba(34, 48, 39, 0.08)"
      }
    }
  },
  plugins: []
};

export default config;
