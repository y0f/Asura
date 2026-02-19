/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './web/templates/**/*.html',
  ],
  theme: {
    extend: {
      colors: {
        brand: '#0080ff',
        surface: {
          DEFAULT: '#09090b',
          50:  '#0e0e12',
          100: '#131318',
          200: '#1a1a22',
          300: '#222230',
        },
        muted: {
          DEFAULT: '#434656',
          light:   '#a7aabc',
        },
        line: {
          DEFAULT: '#1a1a22',
          light:   '#222230',
        },
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Menlo', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
}
