/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        sans: ['Inter', 'Plus Jakarta Sans', 'sans-serif'],
        mono: ['JetBrains Mono', 'Roboto Mono', 'monospace'],
      },
      colors: {
        slate: {
          950: '#020617',
          900: '#0F172A',
          800: '#1E293B',
          700: '#334155',
          400: '#94A3B8',
          100: '#F8FAFC',
        },
        emerald: {
          400: '#34D399',
          500: '#10B981',
        },
        amber: {
          400: '#FBBF24',
          500: '#F59E0B',
        },
        rose: {
          500: '#F43F5E',
        },
      },
    },
  },
  plugins: [],
};
