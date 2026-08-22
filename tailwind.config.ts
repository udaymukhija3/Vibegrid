import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./src/**/*.{ts,tsx,mdx}"],
  theme: {
    extend: {
      colors: {
        ink: "#0d1220",
        "ink-deep": "#070a12",
        paper: "#11162a",
        card: "#f7f2e8",
        cream: "#f7f2e8",
        line: "#353b54",
        lime: "#a3e635",
        amber: "#f8b43a",
        coral: "#ef5350",
        violet: "#7546b8",
        "violet-light": "#b89be8",
        mint: "#a3e635",
        tomato: "#ef5350",
        yolk: "#f8b43a",
        plum: "#7546b8",
        pool: "#b89be8"
      },
      boxShadow: {
        soft: "5px 5px 0 #070a12",
        lift: "7px 7px 0 #070a12",
        tile: "4px 4px 0 #070a12"
      }
    }
  },
  plugins: []
};

export default config;
