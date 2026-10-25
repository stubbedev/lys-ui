<script setup lang="ts">
import { computed } from 'vue'
import { useData, withBase } from 'vitepress'
import index from '../../public/components/index.json'

const { isDark } = useData()
const palette = computed(() => (isDark.value ? { suffix: '', key: 'dark' } : { suffix: '-light', key: 'light' }) as const)
const src = (slug: string) => withBase(`/components/${slug}${palette.value.suffix}.png`)
</script>

<template>
  <div class="widgets">
    <section v-for="w in index" :key="w.slug" :id="w.slug" class="widget">
      <h2>
        <a :href="`#${w.slug}`" class="header-anchor" aria-hidden="true">#</a>
        {{ w.title }}
      </h2>
      <img
        :src="src(w.slug)"
        :width="w[palette.key].width"
        :height="w[palette.key].height"
        :alt="`${w.title}, as lys-ui paints it`"
        loading="lazy"
      />
    </section>
  </div>
</template>
