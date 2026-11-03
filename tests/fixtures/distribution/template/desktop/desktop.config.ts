import { defineDesktopConfig } from '@owdproject/nuxt'

export default defineDesktopConfig({
  apps: ['@owdproject/nuxt-app-about'],
  modules: ['@owdproject/nuxt-module-fs'],
  theme: '@owdproject/nuxt-theme-ui',
})
