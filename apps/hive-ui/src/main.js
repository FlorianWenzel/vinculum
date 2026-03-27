import { createApp } from 'vue'
import PrimeVue from 'primevue/config'
import App from './App.vue'
import { brogPreset } from './primevue-theme'
import './styles.css'
import 'primeicons/primeicons.css'

document.documentElement.classList.add('hive-dark')

createApp(App).use(PrimeVue, {
  theme: {
    preset: brogPreset,
    options: {
      darkModeSelector: '.hive-dark',
      cssLayer: false,
    },
  },
}).mount('#app')
