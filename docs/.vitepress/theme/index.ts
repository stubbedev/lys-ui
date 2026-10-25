import DefaultTheme from 'vitepress/theme'
import Widgets from './Widgets.vue'
import './lys-ui.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('Widgets', Widgets)
  },
}
