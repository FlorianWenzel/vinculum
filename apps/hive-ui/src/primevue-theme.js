import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

export const brogPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '#ecfdf5',
      100: '#d1fae5',
      200: '#a7f3d0',
      300: '#6ee7b7',
      400: '#34d399',
      500: '#22c55e',
      600: '#16a34a',
      700: '#15803d',
      800: '#166534',
      900: '#14532d',
      950: '#052e16',
    },
    focusRing: {
      width: '2px',
      style: 'solid',
      color: '{emerald.400}',
      offset: '2px',
    },
    colorScheme: {
      dark: {
        surface: {
          0: '#111827',
          50: '#0f172a',
          100: '#111827',
          200: '#172033',
          300: '#1f2937',
          400: '#334155',
          500: '#475569',
          600: '#64748b',
          700: '#94a3b8',
          800: '#cbd5e1',
          900: '#e2e8f0',
          950: '#f8fafc',
        },
        primary: {
          color: '#22c55e',
          inverseColor: '{slate.950}',
          hoverColor: '#16a34a',
          activeColor: '#15803d',
        },
        highlight: {
          background: 'rgba(34, 197, 94, 0.18)',
          focusBackground: 'rgba(34, 197, 94, 0.28)',
          color: 'rgba(236, 253, 245, 0.95)',
          focusColor: 'rgba(236, 253, 245, 0.95)',
        },
        formField: {
          background: 'rgba(15, 23, 42, 0.88)',
          disabledBackground: 'rgba(15, 23, 42, 0.66)',
          borderColor: 'rgba(148, 163, 184, 0.2)',
          hoverBorderColor: '{emerald.400}',
          color: 'rgba(241, 245, 249, 0.96)',
          placeholderColor: 'rgba(148, 163, 184, 0.76)',
        },
      },
    },
  },
})
