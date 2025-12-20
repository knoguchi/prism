/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'proxy-bg': '#1e1e1e',
        'proxy-sidebar': '#252526',
        'proxy-border': '#3c3c3c',
        'proxy-text': '#cccccc',
        'proxy-text-dim': '#808080',
        'proxy-accent': '#0e639c',
        'proxy-success': '#4ec9b0',
        'proxy-warning': '#dcdcaa',
        'proxy-error': '#f14c4c',
      },
    },
  },
  plugins: [],
}
