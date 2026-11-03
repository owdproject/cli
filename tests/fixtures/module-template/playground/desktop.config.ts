import { defineDesktopConfig } from '@owdproject/nuxt'

export default defineDesktopConfig({
  theme: '@owdproject/nuxt-theme-paper',
  modules: ['@owdproject/module-template'],
  apps: ['@owdproject/nuxt-app-todo'],
  systemBar: { enabled: true, startButton: true },
})
