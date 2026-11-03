import { defineNuxtPlugin } from 'nuxt/app'
import { defineDesktopApp } from '@owdproject/nuxt/kit/defineDesktopApp'
import configApp from './app.config'

export default defineNuxtPlugin({
  name: 'desktop-app-template-register',
  async setup() {
    if (import.meta.server) return
    await defineDesktopApp(configApp)
  },
})
