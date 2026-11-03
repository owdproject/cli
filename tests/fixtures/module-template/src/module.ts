import { addPlugin, createResolver } from '@nuxt/kit'
import type { NuxtModule } from '@nuxt/schema'
import { defineDesktopModule } from '@owdproject/nuxt/kit'

const module: NuxtModule<any> = defineDesktopModule({
  meta: {
    name: 'desktop-module-template',
    configKey: 'template',
    kind: 'module',
    framework: 'nuxt',
    packageName: '@owdproject/module-template',
    adapter: '@owdproject/nuxt',
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

export default module
