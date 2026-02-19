/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './web/templates/**/*.html',
  ],
  safelist: [
    // Returned by Go template functions at runtime — not literal strings in HTML
    // statusDot, uptimeBarColor
    'bg-emerald-500', 'bg-yellow-500', 'bg-red-500',
    'bg-emerald-400', 'bg-red-400', 'bg-yellow-400', 'bg-gray-500',
    // statusColor, uptimeColor, httpStatusColor, certColor
    'text-emerald-400', 'text-red-400', 'text-yellow-400',
    'text-gray-500', 'text-gray-400', 'text-blue-400',
    // statusBg (multi-class strings)
    'bg-emerald-500/10', 'bg-red-500/10', 'bg-yellow-500/10', 'bg-gray-500/10',
    'border-emerald-500/20', 'border-red-500/20', 'border-yellow-500/20', 'border-gray-500/20',
    // uptimeBarColor no-data
    'bg-muted/20',
  ],
  theme: {
    extend: {
      colors: {
        // Override white to follow theme (dark navy in light mode, white in dark mode)
        white: 'rgb(var(--text-primary-rgb) / <alpha-value>)',
        brand: 'rgb(var(--brand-rgb) / <alpha-value>)',
        surface: {
          DEFAULT: 'rgb(var(--surface-rgb) / <alpha-value>)',
          50:  'rgb(var(--surface-50-rgb) / <alpha-value>)',
          100: 'rgb(var(--surface-100-rgb) / <alpha-value>)',
          200: 'rgb(var(--surface-200-rgb) / <alpha-value>)',
          300: 'rgb(var(--surface-300-rgb) / <alpha-value>)',
        },
        muted: {
          DEFAULT: 'rgb(var(--muted-rgb) / <alpha-value>)',
          light:   'rgb(var(--muted-light-rgb) / <alpha-value>)',
        },
        line: {
          DEFAULT: 'rgb(var(--line-rgb) / <alpha-value>)',
          light:   'rgb(var(--line-light-rgb) / <alpha-value>)',
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
