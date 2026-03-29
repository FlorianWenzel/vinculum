import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  compatibilityDate: '2026-03-28',
  devtools: { enabled: false },
  css: ['~/assets/css/main.css'],
  vite: {
    plugins: [tailwindcss()]
  },
  app: {
    head: {
      htmlAttrs: {
        class: 'dark',
        lang: 'en'
      },
      title: 'VINCULUM | THE AUTONOMOUS TERMINAL',
      meta: [
        {
          name: 'description',
          content:
            'Vinculum is an autonomous terminal for AI-driven software delivery with coordinated agent swarms, built-in platform services, and deployable Helm-based workflows.'
        }
      ],
      link: [
        { rel: 'preconnect', href: 'https://fonts.googleapis.com' },
        { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@300;400;500;600;700;900&family=Inter:wght@300;400;500;600;700&display=swap'
        },
        {
          rel: 'stylesheet',
          href: 'https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:wght,FILL@100..700,0..1&display=swap'
        }
      ]
    }
  },
  nitro: {
    prerender: {
      routes: ['/', '/docs']
    }
  },
  routeRules: {
    '/**': { prerender: true }
  }
})
