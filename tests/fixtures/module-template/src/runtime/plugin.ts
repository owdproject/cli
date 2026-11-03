import { defineNuxtPlugin } from 'nuxt/app'

export default defineNuxtPlugin({
  name: 'owd-plugin-template',
  async setup() {
    if (import.meta.server) return
    console.log('hello world')
  },
})
