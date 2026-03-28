export default defineNuxtConfig({
  compatibilityDate: '2026-03-28',
  devtools: { enabled: false },
  css: ['~/assets/css/main.css'],
  app: {
    head: {
      title: 'Vinculum',
      meta: [
        {
          name: 'description',
          content:
            'Autonomous software delivery on Kubernetes with orchestrated AI drones, built-in Forgejo, Keycloak, and deployable Helm charts.'
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
