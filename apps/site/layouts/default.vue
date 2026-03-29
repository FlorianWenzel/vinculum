<script setup lang="ts">
const route = useRoute()

const desktopNavItems = [
  { key: 'home', label: 'HOME', href: '/' },
  { key: 'roles', label: 'ROLES', href: '/#swarm' },
  { key: 'architecture', label: 'ARCHITECTURE', href: '/#architecture' },
  { key: 'docs', label: 'DOCS', href: '/docs' }
]

const mobileMenuItems = [
  { label: 'Home', kind: 'route', target: '/' },
  { label: 'Roles', kind: 'route', target: '/#swarm' },
  { label: 'Architecture', kind: 'route', target: '/#architecture' },
  { label: 'Install', kind: 'route', target: '/#install' },
  { label: 'Docs', kind: 'route', target: '/docs' }
] as const

const desktopActiveKey = computed(() => {
  if (route.path.startsWith('/docs')) {
    return 'docs'
  }

  return 'home'
})

const installHref = computed(() => route.path.startsWith('/docs') ? '/#install' : '#install')

const menuOpen = ref(false)

async function handleMenuNavigation(item: { kind: 'route'; target: string }) {
  menuOpen.value = false
  await navigateTo(item.target)
}
</script>

<template>
  <div>
    <header>
      <nav class="fixed top-0 z-50 hidden w-full border-b border-[#3b4b37]/20 bg-[#131313]/80 shadow-[0_0_20px_rgba(0,255,65,0.05)] backdrop-blur-xl md:block">
        <div class="flex items-center justify-between px-6 py-4">
          <div class="font-headline text-2xl font-black tracking-[0.3em] text-[#00ff41] uppercase">VINCULUM</div>
          <div class="hidden gap-8 md:flex">
            <a
              v-for="item in desktopNavItems"
              :key="item.key"
              :href="item.href"
              class="font-headline border-b-2 pb-1 text-sm tracking-tight uppercase transition-colors"
              :class="item.key === desktopActiveKey ? 'border-[#00ff41] text-[#00ff41]' : 'border-transparent text-[#84967e] hover:text-[#00ff41]'"
            >
              {{ item.label }}
            </a>
          </div>
          <a
            :href="installHref"
            class="bg-primary-container px-6 py-2 font-headline text-sm font-bold tracking-[0.3em] text-on-primary uppercase transition-all duration-100 hover:shadow-[0_0_15px_#00ff41] active:scale-95"
          >
            INSTALL
          </a>
        </div>
      </nav>

      <div class="fixed top-0 z-50 flex h-14 w-full items-center justify-between bg-gradient-to-b from-[#131313] to-[#1c1b1b] px-4 shadow-[0_0_15px_rgba(0,255,65,0.1)] md:hidden">
        <div class="flex items-center gap-3">
          <button
            type="button"
            class="flex items-center justify-center text-[#00ff41]"
            aria-label="Open mobile menu"
            @click="menuOpen = !menuOpen"
          >
            <span class="material-symbols-outlined">{{ menuOpen ? 'close' : 'menu' }}</span>
          </button>
          <a href="/" class="font-headline text-sm font-bold tracking-[0.3em] text-[#00ff41] uppercase">VINCULUM</a>
        </div>
        <div class="flex items-center gap-4">
          <span class="material-symbols-outlined text-xl text-[#00ff41]">terminal</span>
        </div>
      </div>

      <Transition
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="menuOpen" class="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm md:hidden" @click="menuOpen = false" />
      </Transition>

      <Transition
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="-translate-y-4 opacity-0"
        enter-to-class="translate-y-0 opacity-100"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="translate-y-0 opacity-100"
        leave-to-class="-translate-y-4 opacity-0"
      >
        <div v-if="menuOpen" class="fixed top-14 right-4 left-4 z-50 border border-outline-variant/30 bg-surface-container-lowest/95 p-4 shadow-[0_0_25px_rgba(0,255,65,0.08)] backdrop-blur-xl md:hidden">
          <div class="mb-4 border-b border-outline-variant/20 pb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">
            Navigation
          </div>
          <div class="space-y-2">
            <button
              v-for="item in mobileMenuItems"
              :key="item.label"
              type="button"
              class="flex w-full items-center justify-between border border-outline-variant/20 bg-surface-container px-4 py-3 text-left font-headline text-sm font-bold tracking-[0.16em] text-on-surface uppercase"
              @click="handleMenuNavigation(item)"
            >
              <span>{{ item.label }}</span>
              <span class="material-symbols-outlined text-base text-primary-container">arrow_forward</span>
            </button>
          </div>
        </div>
      </Transition>
    </header>

    <slot />
  </div>
</template>
