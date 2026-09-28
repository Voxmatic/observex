/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        sans:  ['"DM Sans"', 'system-ui', 'sans-serif'],
        mono:  ['"JetBrains Mono"', '"Fira Code"', 'monospace'],
        display: ['"Syne"', 'system-ui', 'sans-serif'],
      },
      colors: {
        surface: {
          0:  'hsl(220 13% 9%)',
          1:  'hsl(220 11% 12%)',
          2:  'hsl(220 10% 16%)',
          3:  'hsl(220 9% 21%)',
          4:  'hsl(220 8% 28%)',
        },
        brand: {
          DEFAULT: 'hsl(217 91% 60%)',
          dim:     'hsl(217 60% 40%)',
          glow:    'hsl(217 91% 70%)',
        },
        ok:   'hsl(142 71% 45%)',
        warn: 'hsl(38 92% 50%)',
        crit: 'hsl(0 84% 60%)',
        info: 'hsl(199 89% 48%)',
      },
      animation: {
        'pulse-slow': 'pulse 3s cubic-bezier(0.4,0,0.6,1) infinite',
        'fade-in':    'fadeIn 0.15s ease-out',
        'slide-up':   'slideUp 0.2s ease-out',
      },
      keyframes: {
        fadeIn:  { from: { opacity: '0' }, to: { opacity: '1' } },
        slideUp: { from: { opacity: '0', transform: 'translateY(6px)' }, to: { opacity: '1', transform: 'translateY(0)' } },
      },
    },
  },
  plugins: [],
}
