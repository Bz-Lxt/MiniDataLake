/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        bg: "#071018",
        panel: "#0d1824",
        panel2: "#122132",
        line: "#1e3348",
        ink: "#d7e6f2",
        mute: "#7f9aaf",
        amber: "#f0a202",
        cyan: "#3ecfcf",
        rose: "#ef6b7b",
        ok: "#3dd68c",
      },
      fontFamily: {
        sans: ["IBM Plex Sans", "ui-sans-serif", "system-ui"],
        mono: ["IBM Plex Mono", "ui-monospace", "monospace"],
      },
    },
  },
  plugins: [],
};
