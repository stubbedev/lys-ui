import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'lys-ui',
  description: 'A pure-Go Wayland widget toolkit with shadcn/ui components.',
  base: '/lys-ui/',
  cleanUrls: true,
  lastUpdated: true,
  srcExclude: ['node_modules/**'],

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/lys-ui/lys-ui.svg' }],
  ],

  markdown: {
    theme: {
      light: 'github-light',
      dark: 'github-dark',
    },
  },

  themeConfig: {
    logo: '/lys-ui.svg',

    nav: [
      { text: 'Guide', link: '/guide/getting-started', activeMatch: '^/guide/' },
      { text: 'Widgets', link: '/widgets' },
      { text: 'Reference', link: '/architecture', activeMatch: '^/(architecture|application-model|components|threading|input-model|css|appearance|icons|a11y|inspector|completeness)' },
    ],

    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'Widgets', link: '/widgets' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Architecture', link: '/architecture' },
          { text: 'Application model', link: '/application-model' },
          { text: 'Components', link: '/components' },
          { text: 'Threading', link: '/threading' },
          { text: 'Input model', link: '/input-model' },
          { text: 'CSS', link: '/css' },
          { text: 'Appearance', link: '/appearance' },
          { text: 'Icons', link: '/icons' },
          { text: 'Accessibility', link: '/a11y' },
          { text: 'Inspector', link: '/inspector' },
          { text: 'Completeness', link: '/completeness' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/stubbedev/lys-ui' },
    ],

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright 2026 lys-ui contributors',
    },

    editLink: {
      pattern: 'https://github.com/stubbedev/lys-ui/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    outline: {
      level: [2, 3],
    },

    search: {
      provider: 'local',
    },
  },
})
