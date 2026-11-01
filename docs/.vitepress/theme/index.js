import Theme from 'lys-docs-theme/theme'
import Widgets from './Widgets.vue'
import './widgets.css'

export default {
  extends: Theme,
  enhanceApp({ app }) {
    app.component('Widgets', Widgets)
  },
}
