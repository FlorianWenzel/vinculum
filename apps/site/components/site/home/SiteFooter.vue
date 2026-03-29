<script setup lang="ts">
defineProps<{
  footerLinks: Array<{ label: string; href: string }>
}>()
</script>

<template>
  <footer class="w-full border-t border-[#3b4b37]/20 bg-[#131313] py-12">
    <div class="flex flex-col items-center justify-between gap-8 px-10 md:flex-row">
      <div class="flex flex-col items-center gap-2 md:items-start">
        <div class="font-body text-[10px] font-bold tracking-[0.3em] text-[#00ff41] uppercase">VINCULUM</div>
        <p class="font-body text-[10px] tracking-[0.3em] text-[#00ff41]/80 uppercase">Released under the MIT License. SYSTEM STATUS: OPERATIONAL.</p>
      </div>
      <div class="flex gap-8">
        <a v-for="link in footerLinks" :key="link.label" :href="link.href" class="font-body text-[10px] tracking-[0.3em] text-[#69df5c]/60 uppercase transition-opacity duration-300 hover:text-[#00ff41]">
          {{ link.label }}
        </a>
      </div>
      <div class="flex gap-4">
        <span class="material-symbols-outlined text-sm text-primary-container/40">hub</span>
        <span class="material-symbols-outlined text-sm text-primary-container/40">memory</span>
        <span class="material-symbols-outlined text-sm text-primary-container/40">terminal</span>
      </div>
    </div>
  </footer>
</template>
