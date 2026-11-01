import { lysConfig } from 'lys-docs-theme/config'

export default lysConfig({
  name: 'lys-ui',
  description: 'A pure-Go Wayland widget toolkit.',
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
        { text: 'Demos', link: '/guide/demos' },
        { text: 'Development', link: '/guide/development' },
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
})
