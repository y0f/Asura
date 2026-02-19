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
