/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        theme: {
          bg: 'var(--color-primary, #ffffff)',
          accent: 'var(--color-accent, #dc2626)',
          accentHover: 'var(--color-accent-hover, #b91c1c)',
          accentLight: 'var(--color-accent-light, #fef2f2)',
        },
      },
    },
  },
  plugins: [],
}
