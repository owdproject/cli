import { addPlugin, createResolver } from '@nuxt/kit'
import { defineDesktopModule } from '@owdproject/core/kit/authoring'

export default defineDesktopModule({
  meta: {
    name: 'desktop-module-template',
    configKey: 'template',
  },
  setup() {
    const { resolve } = createResolver(import.meta.url)

    addPlugin({
      src: resolve('./runtime/plugin'),
      mode: 'client',
    })

    /*
    addImportsDir(resolve('./runtime/composables'))
    addImportsDir(resolve('./runtime/stores'))
    addImportsDir(resolve('./runtime/utils'))
    */
  },
})
