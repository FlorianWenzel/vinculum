<script setup lang="ts">
const props = defineProps<{
  mobileDrones: Array<{ title: string; subtitle: string; icon: string; accent: string; iconColor: string; subtitleColor: string; bars: string[]; dimmed?: boolean; pulse?: boolean }>
  mobileWorkflow: Array<{ number: string; title: string; text: string }>
  mobileStack: Array<{ icon: string; label: string }>
  footerLinks: Array<{ label: string; href: string }>
}>()

const menuOpen = ref(false)

const mobileMenuLinks = [
  { label: 'Home', kind: 'scroll', target: 'top' },
  { label: 'Drone Roles', kind: 'scroll', target: 'mobile-roles' },
  { label: 'Workflow', kind: 'scroll', target: 'mobile-workflow' },
  { label: 'Components', kind: 'scroll', target: 'mobile-components' },
  { label: 'Install', kind: 'scroll', target: 'mobile-install' },
  { label: 'Docs', kind: 'route', target: '/docs' }
] as const

const bottomNavItems = [
  { icon: 'home', label: 'Home', kind: 'scroll', target: 'top' },
  { icon: 'smart_toy', label: 'Roles', kind: 'scroll', target: 'mobile-roles' },
  { icon: 'account_tree', label: 'Docs', kind: 'route', target: '/docs' },
  { icon: 'memory', label: 'Install', kind: 'scroll', target: 'mobile-install' }
] as const

function scrollToTarget(target: string) {
  if (target === 'top') {
    window.scrollTo({ top: 0, behavior: 'smooth' })
    return
  }

  document.getElementById(target)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

async function handleNavigation(item: { kind: 'scroll' | 'route'; target: string }) {
  menuOpen.value = false

  if (item.kind === 'route') {
    await navigateTo(item.target)
    return
  }

  scrollToTarget(item.target)
}
</script>

<template>
  <div class="md:hidden overflow-x-hidden bg-background pb-20 text-on-background">
    <header class="fixed top-0 z-50 flex h-14 w-full items-center justify-between bg-gradient-to-b from-[#131313] to-[#1c1b1b] px-4 shadow-[0_0_15px_rgba(0,255,65,0.1)]">
      <div class="flex items-center gap-3">
        <button
          type="button"
          class="flex items-center justify-center text-[#00ff41]"
          aria-label="Open mobile menu"
          @click="menuOpen = !menuOpen"
        >
          <span class="material-symbols-outlined">{{ menuOpen ? 'close' : 'menu' }}</span>
        </button>
        <h1 class="font-headline text-sm font-bold tracking-[0.3em] text-[#00ff41] uppercase">VINCULUM</h1>
      </div>
      <div class="flex items-center gap-4">
        <span class="material-symbols-outlined text-xl text-[#00ff41]">terminal</span>
      </div>
    </header>

    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="menuOpen" class="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm" @click="menuOpen = false" />
    </Transition>

    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="-translate-y-4 opacity-0"
      enter-to-class="translate-y-0 opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="translate-y-0 opacity-100"
      leave-to-class="-translate-y-4 opacity-0"
    >
      <div v-if="menuOpen" class="fixed top-14 right-4 left-4 z-50 border border-outline-variant/30 bg-surface-container-lowest/95 p-4 shadow-[0_0_25px_rgba(0,255,65,0.08)] backdrop-blur-xl">
        <div class="mb-4 border-b border-outline-variant/20 pb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">
          Mobile Navigation
        </div>
        <div class="space-y-2">
          <button
            v-for="item in mobileMenuLinks"
            :key="item.label"
            type="button"
            class="flex w-full items-center justify-between border border-outline-variant/20 bg-surface-container px-4 py-3 text-left font-headline text-sm font-bold tracking-[0.16em] text-on-surface uppercase"
            @click="handleNavigation(item)"
          >
            <span>{{ item.label }}</span>
            <span class="material-symbols-outlined text-base text-primary-container">arrow_forward</span>
          </button>
        </div>
      </div>
    </Transition>

    <main class="space-y-8 px-4 pt-14">
      <section class="relative flex flex-col items-center overflow-hidden py-12 text-center">
        <div class="absolute inset-0 pointer-events-none opacity-10 bg-[url('https://www.transparenttextures.com/patterns/carbon-fibre.png')]" />
        <div class="absolute -top-10 -right-10 h-40 w-40 rounded-full bg-primary-container/5 blur-3xl" />
        <div class="relative z-10 space-y-6">
          <div class="inline-flex items-center gap-2 border-l-2 border-primary-container bg-surface-container-high px-3 py-1">
            <span class="material-symbols-outlined text-[10px] text-primary-container" style="font-variation-settings: 'FILL' 1;">circle</span>
            <span class="font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">PUBLIC_INSTALL: READY</span>
          </div>
          <h2 class="font-headline text-4xl font-extrabold leading-none tracking-tighter text-primary-container">
            AUTONOMOUS SOFTWARE DELIVERY. <br>
            <span class="text-white/90">DEPLOYED WITH HELM.</span>
          </h2>
          <p class="mx-auto max-w-xs font-body text-sm leading-relaxed text-secondary opacity-80">
            Vinculum provides a Kubernetes-native platform for drone-based implementation, review, testing, and deployment workflows.
          </p>
          <div class="flex flex-col gap-3 pt-4">
            <a href="#mobile-install" class="bg-primary-container px-8 py-4 font-label text-xs font-bold tracking-[0.3em] text-on-primary uppercase transition-all hover:shadow-[0_0_15px_rgba(0,255,65,0.4)]">
              INSTALL_WITH_HELM
            </a>
            <NuxtLink to="/docs" class="border border-outline px-8 py-4 font-label text-xs tracking-[0.3em] text-secondary uppercase transition-all hover:bg-surface-variant">
              READ_DOCS
            </NuxtLink>
          </div>
        </div>
      </section>

      <section id="mobile-roles" class="space-y-4 scroll-mt-20">
        <div class="flex items-baseline justify-between border-b border-outline-variant pb-2">
          <h3 class="font-headline text-lg font-bold tracking-[0.2em] text-primary-container uppercase">DRONE_ROLES</h3>
          <span class="font-label text-[10px] text-outline">PUBLIC</span>
        </div>
        <div class="grid grid-cols-1 gap-3">
          <article
            v-for="drone in mobileDrones"
            :key="drone.title"
            class="flex items-center justify-between border-l-2 bg-surface-container-low p-4"
            :class="drone.accent"
          >
            <div class="flex items-center gap-4" :class="drone.dimmed ? 'opacity-70' : ''">
              <div class="flex h-10 w-10 items-center justify-center bg-surface-container" :class="drone.iconColor">
                <span class="material-symbols-outlined text-2xl">{{ drone.icon }}</span>
              </div>
              <div>
                <div class="font-headline text-sm font-bold tracking-[0.16em]">{{ drone.title }}</div>
                <div class="font-label text-[10px] uppercase" :class="drone.subtitleColor">{{ drone.subtitle }}</div>
              </div>
            </div>
            <div class="flex gap-1" :class="drone.pulse ? 'animate-pulse' : ''">
              <div v-for="(bar, index) in drone.bars" :key="`${drone.title}-${index}`" class="h-4 w-1" :class="bar" />
            </div>
          </article>
        </div>
      </section>

      <section id="mobile-workflow" class="space-y-6 scroll-mt-20">
        <h3 class="border-b border-outline-variant pb-2 font-headline text-lg font-bold tracking-[0.2em] text-primary-container uppercase">DELIVERY_WORKFLOW</h3>
        <div class="relative grid grid-cols-1 gap-6">
          <div class="absolute top-4 bottom-4 left-[19px] w-px bg-outline-variant/30" />
          <div v-for="step in mobileWorkflow" :key="step.number" class="relative flex gap-6">
            <div class="z-10 flex h-10 w-10 items-center justify-center border border-primary-container bg-surface font-headline font-bold text-primary-container">
              {{ step.number }}
            </div>
            <div class="flex-1 space-y-1">
              <div class="font-headline text-xs font-bold tracking-[0.16em] text-on-surface uppercase">{{ step.title }}</div>
              <p class="font-body text-xs leading-relaxed text-outline">{{ step.text }}</p>
            </div>
          </div>
        </div>
      </section>

      <section id="mobile-components" class="space-y-6 bg-surface-container-low p-6 scroll-mt-20">
        <h3 class="text-center font-headline text-sm font-bold tracking-[0.2em] text-primary-container uppercase">PLATFORM_COMPONENTS</h3>
        <div class="grid grid-cols-2 gap-px bg-outline-variant/20">
          <div v-for="item in mobileStack" :key="item.label" class="group flex flex-col items-center gap-2 bg-surface-container-low p-4">
            <span class="material-symbols-outlined text-primary-container/60 transition-colors group-hover:text-primary-container">{{ item.icon }}</span>
            <span class="font-label text-[10px] tracking-tight text-outline uppercase">{{ item.label }}</span>
          </div>
        </div>
      </section>

      <section id="mobile-install" class="space-y-4 scroll-mt-20">
        <h3 class="border-b border-outline-variant pb-2 font-headline text-lg font-bold tracking-[0.2em] text-primary-container uppercase">INSTALL_WITH_HELM</h3>
        <div class="relative overflow-x-auto border border-outline-variant bg-black p-4 font-mono text-[11px] leading-tight shadow-2xl">
          <div class="absolute top-2 right-2 flex gap-1">
            <div class="h-1 w-1 bg-primary-container/40" />
            <div class="h-1 w-1 bg-primary-container/40" />
            <div class="h-1 w-1 bg-primary-container/40" />
          </div>
          <div class="mb-2 text-secondary opacity-50">// PUBLIC INSTALL PATH</div>
          <div class="flex gap-2"><span class="text-primary-container">$</span><span>helm repo add vinculum https://florianwenzel.github.io/vinculum</span></div>
          <div class="mt-2 flex gap-2"><span class="text-primary-container">$</span><span>helm repo update</span></div>
          <div class="mt-2 flex gap-2"><span class="text-primary-container">$</span><span>helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace</span></div>
          <div class="mt-4 flex items-center text-primary-container">
            <span>INITIALIZING_STACK...</span>
            <span class="terminal-block" />
          </div>
        </div>
        <div class="grid grid-cols-1 gap-2">
          <div class="flex items-center justify-between bg-surface-container-highest/30 p-3">
            <span class="font-label text-[10px] text-outline uppercase">Package source: GitHub Pages</span>
            <span class="material-symbols-outlined text-xs text-primary-container">link</span>
          </div>
        </div>
      </section>

      <footer class="border-t border-outline-variant/20 px-2 pt-8 pb-12 opacity-70">
        <div class="mb-4 flex flex-wrap gap-2">
          <a
            v-for="link in props.footerLinks"
            :key="link.label"
            :href="link.href"
            class="border border-outline-variant/20 bg-surface-container px-3 py-2 font-label text-[10px] tracking-[0.2em] text-outline uppercase transition-colors hover:text-primary-container"
          >
            {{ link.label }}
          </a>
        </div>
        <div class="flex items-center justify-between">
          <span class="font-label text-[9px] tracking-[0.2em] uppercase">Public_Docs_v1</span>
          <span class="font-label text-[9px] tracking-[0.2em] uppercase">MIT LICENSE</span>
        </div>
      </footer>
    </main>

    <nav class="safe-bottom fixed bottom-0 left-0 z-50 flex h-16 w-full items-center justify-around bg-[#131313]/90 shadow-[0_-4px_20px_rgba(0,255,65,0.05)] backdrop-blur-md">
      <button
        v-for="item in bottomNavItems"
        :key="item.label"
        type="button"
        class="flex flex-1 flex-col items-center justify-center text-[#84967e] transition-transform hover:text-[#ebffe2] active:scale-110"
        @click="handleNavigation(item)"
      >
        <span class="material-symbols-outlined" :class="item.label === 'Home' ? 'text-[#00ff41] drop-shadow-[0_0_8px_rgba(0,255,65,0.8)]' : ''">{{ item.icon }}</span>
        <span class="mt-1 font-label text-[9px] tracking-[0.2em] uppercase">{{ item.label }}</span>
      </button>
    </nav>
  </div>
</template>
