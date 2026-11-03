import { defineDesktopConfig } from '@owdproject/nuxt'

export default defineDesktopConfig({
  theme: '@owdproject/nuxt-theme-paper',
  apps: ['@owdproject/app-template'],
  modules: [],
  systemBar: {
    enabled: true,
    startButton: true,
  },
})
