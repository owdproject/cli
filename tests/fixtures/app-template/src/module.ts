import {
  defineNuxtModule,
  createResolver,
  addComponentsDir,
  addPlugin,
} from '@nuxt/kit'
import { registerTailwindPath } from '@owdproject/nuxt-kit-tailwind/kit/registerTailwindPath'

export default defineNuxtModule({
  meta: {
    name: 'desktop-app-template',
    configKey: 'template',
    kind: 'app',
    framework: 'nuxt',
    packageName: '@owdproject/app-template',
    adapter: '@owdproject/nuxt',
  },
  async setup(options, nuxt) {
    const { resolve } = createResolver(import.meta.url)

    // add components

    addComponentsDir({
      path: resolve('./runtime/components'),
    })

    // add plugins

    addPlugin(resolve('./runtime/plugin'))

    // configure tailwind

    registerTailwindPath(
      nuxt,
      resolve('./runtime/components/**/*.{vue,mjs,ts}'),
    )
  },
})
