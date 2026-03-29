<script setup lang="ts">
defineProps<{
  id: string
  title: string
  points: string[]
}>()
</script>

<template>
  <article :id="id" class="border border-outline-variant/20 bg-surface-container-low p-8">
    <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">{{ id }}</div>
    <h2 class="font-headline text-3xl font-black tracking-tight uppercase md:text-4xl">{{ title }}</h2>
    <ul class="mt-6 space-y-4 text-sm leading-7 text-on-surface-variant md:text-base">
      <li v-for="point in points" :key="point" class="border-l-2 border-primary-container/50 pl-4">
        {{ point }}
      </li>
    </ul>
  </article>
</template>
