<script setup lang="ts">
defineProps<{
  desktopNav: Array<{ label: string; href: string; active?: boolean }>
  droneCards: Array<{ title: string; unit: string; icon: string; description: string; status?: string; fill?: boolean; span?: string }>
  workflowSteps: Array<{ number: string; title: string; text: string; active?: boolean }>
  stackItems: Array<{ number: string; title: string; note: string }>
}>()
</script>

<template>
  <div class="hidden md:block">
    <nav class="fixed top-0 z-50 w-full border-b border-[#3b4b37]/20 bg-[#131313]/80 shadow-[0_0_20px_rgba(0,255,65,0.05)] backdrop-blur-xl">
      <div class="flex items-center justify-between px-6 py-4">
        <div class="font-headline text-2xl font-black tracking-[0.3em] text-[#00ff41] uppercase">VINCULUM</div>
        <div class="hidden gap-8 md:flex">
          <a
            v-for="item in desktopNav"
            :key="item.label"
            :href="item.href"
            class="font-headline border-b-2 pb-1 text-sm tracking-tight uppercase transition-colors"
            :class="item.active ? 'border-[#00ff41] text-[#00ff41]' : 'border-transparent text-[#84967e] hover:text-[#00ff41]'"
          >
            {{ item.label }}
          </a>
        </div>
        <a
          href="#install"
          class="bg-primary-container px-6 py-2 font-headline text-sm font-bold tracking-[0.3em] text-on-primary uppercase transition-all duration-100 hover:shadow-[0_0_15px_#00ff41] active:scale-95"
        >
          INSTALL
        </a>
      </div>
    </nav>

    <main class="overflow-x-hidden pt-24">
      <section class="relative flex min-h-[921px] flex-col items-center justify-center overflow-hidden px-6">
        <div class="mask-radial absolute inset-0 z-0 opacity-20">
          <div
            class="h-full w-full bg-cover bg-center"
            style="background-image: url('https://images.unsplash.com/photo-1635070041078-e363dbe005cb?auto=format&fit=crop&q=80&w=2000')"
          />
        </div>

        <div class="relative z-10 max-w-5xl text-center">
          <div class="mb-6 inline-block border border-primary-container/30 bg-primary-container/5 px-3 py-1 font-label text-xs tracking-[0.3em] text-primary-container uppercase">
            Platform Overview // Public
          </div>
          <h1 class="mb-6 font-headline text-6xl leading-none font-black tracking-tighter uppercase md:text-8xl">
            Autonomous Software Delivery.<br>
            <span class="text-primary-container">Built for Kubernetes.</span>
          </h1>
          <p class="mx-auto mb-10 max-w-3xl font-body text-lg leading-relaxed tracking-tight text-on-surface-variant md:text-2xl">
            Vinculum coordinates requirements, task planning, coding, review, testing, and deployment workflows through a Kubernetes-native control plane and a set of specialized drones.
          </p>
          <div class="flex flex-col justify-center gap-4 md:flex-row">
            <a
              href="#install"
              class="energy-flux px-10 py-4 font-headline text-lg font-black tracking-[0.18em] text-on-primary uppercase transition-all hover:shadow-[0_0_25px_rgba(0,255,65,0.4)]"
            >
              Install With Helm
            </a>
            <NuxtLink
              to="/docs"
              class="border border-outline px-10 py-4 font-headline text-lg font-bold tracking-[0.18em] text-secondary uppercase transition-all hover:bg-surface-variant"
            >
              Read Documentation
            </NuxtLink>
          </div>
        </div>

        <div class="absolute right-10 bottom-10 hidden w-80 border border-outline-variant/30 bg-surface-container-lowest/80 p-4 font-mono text-[10px] text-secondary/70 backdrop-blur-md lg:block">
          <div class="mb-2 flex justify-between border-b border-outline-variant/20 pb-1">
            <span class="text-primary-container">RUNTIME_LOG</span>
            <span>LOC: 0x882F</span>
          </div>
          <div class="space-y-1">
            <p>&gt; loading platform surfaces...</p>
            <p>&gt; coder_drone-01 online</p>
            <p>&gt; reviewer_drone-04 online</p>
            <p>&gt; deployer_drone-02 standby</p>
            <p class="terminal-cursor text-primary-container">&gt; awaiting deployment command</p>
          </div>
        </div>
      </section>

      <section id="swarm" class="mx-auto max-w-7xl px-6 py-24 md:px-10">
        <div class="mb-16 flex flex-col items-end justify-between gap-4 md:flex-row">
          <div class="max-w-xl">
            <h2 class="mb-4 font-headline text-4xl font-black tracking-tighter uppercase md:text-5xl">Drone Roles</h2>
            <p class="font-body text-on-surface-variant">Each drone is responsible for a distinct part of the delivery loop, from implementation and review through testing and release preparation.</p>
          </div>
          <div class="font-label text-sm tracking-[0.2em] text-primary-container">[ PLATFORM_READY: TRUE ]</div>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
          <article
            v-for="card in droneCards"
            :key="card.title"
            class="border border-outline-variant/20 bg-surface-container-low p-8 transition-all hover:border-primary-container/50"
            :class="card.span"
          >
            <div class="mb-12 flex items-start justify-between">
              <span
                class="material-symbols-outlined text-4xl text-primary-container"
                :style="card.fill ? 'font-variation-settings: \'FILL\' 1;' : ''"
              >
                {{ card.icon }}
              </span>
              <span class="font-label text-[10px] tracking-[0.3em] text-outline uppercase">Unit: {{ card.unit }}</span>
            </div>
            <h3 class="mb-4 font-headline text-2xl font-bold tracking-tight uppercase">{{ card.title }}</h3>
            <p class="mb-6 text-sm leading-relaxed text-on-surface-variant">{{ card.description }}</p>
            <div v-if="card.status" class="border-l-2 border-primary-container bg-surface-container px-3 py-2 font-mono text-[10px] text-secondary">
              {{ card.status }}
            </div>
          </article>

          <article class="md:col-span-4 flex flex-col items-center gap-8 border border-outline-variant/20 bg-surface-container-low p-8 transition-all hover:border-primary-container/50 md:flex-row">
            <div class="bg-primary-container/10 p-6">
              <span class="material-symbols-outlined text-5xl text-primary-container" style="font-variation-settings: 'FILL' 1;">
                rocket_launch
              </span>
            </div>
            <div class="flex-grow">
              <div class="mb-2 flex justify-between gap-4">
                <h3 class="font-headline text-2xl font-bold tracking-tight uppercase">Deployer</h3>
                <span class="font-label text-[10px] tracking-[0.3em] text-outline uppercase">Unit: 0x04</span>
              </div>
              <p class="max-w-2xl text-sm leading-relaxed text-on-surface-variant">
                Coordinates deployment-oriented workflows and environment automation so validated changes can move toward running systems.
              </p>
            </div>
            <div class="w-full md:w-auto">
              <div class="flex gap-2">
                <div class="h-1 w-12 bg-primary-container" />
                <div class="h-1 w-12 bg-primary-container" />
                <div class="h-1 w-12 bg-outline-variant" />
              </div>
            </div>
          </article>
        </div>
      </section>

      <section class="bg-surface-container-low py-24">
        <div class="mx-auto max-w-7xl px-6">
          <div class="mb-20 text-center">
            <h2 class="mb-4 font-headline text-4xl font-black tracking-tighter uppercase md:text-5xl">Delivery Workflow</h2>
            <div class="mx-auto h-1 w-24 bg-primary-container" />
          </div>

          <div class="relative grid grid-cols-1 gap-0 md:grid-cols-4">
            <div class="absolute top-1/2 left-0 z-0 hidden h-px w-full -translate-y-1/2 bg-outline-variant/30 md:block" />
            <div v-for="step in workflowSteps" :key="step.number" class="relative z-10 flex flex-col items-center p-8 text-center">
              <div
                class="mb-6 flex h-16 w-16 items-center justify-center font-headline text-2xl font-black shadow-[0_0_15px_rgba(0,255,65,0.2)]"
                :class="step.active ? 'bg-primary-container text-on-primary shadow-[0_0_25px_rgba(0,255,65,0.5)]' : 'border border-primary-container bg-surface-container text-on-surface'"
              >
                {{ step.number }}
              </div>
              <h4 class="mb-2 font-headline font-bold tracking-tight uppercase" :class="step.active ? 'text-primary-container' : ''">{{ step.title }}</h4>
              <p class="font-body text-xs tracking-[0.16em] text-on-surface-variant uppercase">{{ step.text }}</p>
            </div>
          </div>
        </div>
      </section>

      <section id="architecture" class="px-6 py-24">
        <div class="mx-auto flex max-w-7xl flex-col items-center gap-16 lg:flex-row">
          <div class="w-full lg:w-1/2">
            <h2 class="mb-8 font-headline text-4xl font-black tracking-tighter uppercase md:text-5xl">Platform Components</h2>
            <p class="mb-10 text-lg leading-relaxed text-on-surface-variant">
              Vinculum combines operator logic, browser tooling, identity, code hosting, and worker runtimes into one deployable platform.
            </p>
            <div class="space-y-4">
              <div v-for="item in stackItems" :key="item.number" class="flex items-center gap-4 border-l-4 border-primary-container bg-surface-container p-4">
                <span class="font-headline text-secondary font-black">{{ item.number }}</span>
                <span class="font-headline font-bold uppercase">{{ item.title }}</span>
                <span class="ml-auto text-xs text-outline uppercase">{{ item.note }}</span>
              </div>
            </div>
          </div>

          <div class="relative w-full border border-outline-variant/30 bg-surface-container-high p-4 lg:w-1/2">
            <div class="absolute -top-2 -left-2 h-4 w-4 bg-primary-container" />
            <div class="absolute -right-2 -bottom-2 h-4 w-4 bg-primary-container" />
            <img
              class="w-full grayscale contrast-125 brightness-75 mix-blend-screen"
              src="https://lh3.googleusercontent.com/aida-public/AB6AXuCZzTKmDAHa3tR2lLnQr4jNLA6cMabQDVdx3nprXhvDfZ9jpEipny3EARfPJnYJJRZ-I9FAYLkV6xXDC311np_VEgk0J49O_1cSdT8ZR0QszSs7g6jjhGQL3H2UEmQ6PM2C2Wwk6CBL1BMbb8xIHkfT3oPsQ_P7Jy6LwOKhKbs_uW_GK77-_KTcIv_XnUpb4jjdwZScI5UYpBCadYnvl0NxyOTIBGGrsjPLHTMizshPrp9A0FDanzgoIOHu3fjxoTHVvqM61uL6Dw"
              alt="Technical network diagram with glowing green nodes and interconnected server lines against a dark blueprint background"
            >
            <div class="mt-4 bg-surface-container-lowest p-4 font-mono text-xs text-secondary/60">
              PLATFORM_COMPONENTS: ONLINE // RELEASE_CHANNEL: PUBLIC
            </div>
          </div>
        </div>
      </section>

      <section id="install" class="border-y border-outline-variant/10 bg-surface-container-lowest py-24">
        <div class="mx-auto max-w-4xl px-6">
          <div class="mb-12 text-center">
            <h2 class="mb-2 font-headline text-4xl font-black tracking-tighter uppercase">Install With Helm</h2>
            <p class="font-body text-sm tracking-[0.2em] text-on-surface-variant uppercase">Deploy the published chart from the GitHub-hosted Helm repository</p>
          </div>
          <div class="overflow-hidden border border-outline-variant/40 bg-[#0e0e0e]">
            <div class="flex gap-2 border-b border-outline-variant/40 bg-surface-container p-3">
              <div class="h-3 w-3 bg-error/50" />
              <div class="h-3 w-3 bg-primary-container/50" />
              <div class="h-3 w-3 bg-tertiary-container/50" />
            </div>
            <div class="space-y-4 p-8 font-mono text-sm md:text-base">
              <div class="flex gap-4"><span class="text-outline">$</span><span>helm repo add vinculum https://florianwenzel.github.io/vinculum</span></div>
              <div class="flex gap-4"><span class="text-outline">$</span><span>helm repo update</span></div>
              <div class="flex gap-4"><span class="text-outline">$</span><span class="font-bold text-primary-container">helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace</span></div>
            </div>
          </div>
          <div class="mt-10 flex justify-center">
            <NuxtLink to="/docs" class="flex items-center gap-2 text-primary-container transition-all hover:gap-4">
              <span class="font-headline font-bold tracking-[0.2em] uppercase">Open Installation Guide</span>
              <span class="material-symbols-outlined">arrow_forward</span>
            </NuxtLink>
          </div>
        </div>
      </section>
    </main>
  </div>
</template>
