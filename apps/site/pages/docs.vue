<script setup lang="ts">
const installSteps = [
  'helm repo add vinculum https://florianwenzel.github.io/vinculum',
  'helm repo update',
  'helm install vinculum vinculum/vinculum -n vinculum-system --create-namespace'
]

const installNotes = [
  'The umbrella chart installs PostgreSQL, Keycloak, Forgejo, Vinculum Infra, the orchestrator, and Hive UI.',
  'Default first-party image tags follow the chart appVersion unless you override them in values.',
  'Use your own ingress, hostname, storage, and identity settings through Helm values for your environment.'
]

const architectureCards = [
  {
    title: 'Hive UI',
    text: 'Browser control surface for drones, repositories, requirements, tasks, reviews, and operational visibility.'
  },
  {
    title: 'Orchestrator',
    text: 'Go service and Kubernetes operator that manages CRDs, scheduling, task state, and drone execution.'
  },
  {
    title: 'Vinculum Infra',
    text: 'Bootstrap and reconciliation service for shared platform setup across Keycloak and Forgejo.'
  },
  {
    title: 'Vinculum Code',
    text: 'Forgejo-based code hosting, pull requests, repository automation, and access management.'
  },
  {
    title: 'Identity',
    text: 'Keycloak-backed OIDC layer for browser sign-in and platform-to-platform authentication.'
  },
  {
    title: 'Drone Runtime',
    text: 'Containerized worker runtime built around OpenCode plus a control API for isolated task execution.'
  }
]

const workflowSteps = [
  {
    step: '01',
    title: 'Define requirements',
    text: 'Create requirements through the UI or API so work starts from durable product intent instead of disposable chat state.'
  },
  {
    step: '02',
    title: 'Decompose into tasks',
    text: 'The orchestrator turns requirements into technical tasks and assigns available drones based on role and access.'
  },
  {
    step: '03',
    title: 'Execute in pods',
    text: 'Drones run in isolated containers with repository access, instructions, provider auth, and execution controls.'
  },
  {
    step: '04',
    title: 'Review and validate',
    text: 'Review resources, automated checks, and test workflows close the loop before changes are merged or promoted.'
  }
]

const apiResources = [
  'POST /api/drones',
  'POST /api/repositories',
  'POST /api/accesses',
  'POST /api/requirements',
  'POST /api/tasks',
  'POST /api/reviews'
]

const publishedArtifacts = [
  'Helm chart repository: https://florianwenzel.github.io/vinculum',
  'OCI chart namespace: oci://ghcr.io/florianwenzel/helm',
  'Container image namespace: ghcr.io/florianwenzel/*',
  'Source repository: https://github.com/florianwenzel/vinculum'
]

const docsSections = [
  {
    id: 'deployment',
    title: 'Deployment model',
    points: [
      'Charts under `helm/` are packaged and published through GitHub Actions to both GHCR and GitHub Pages.',
      'The public install path uses the `vinculum/vinculum` umbrella chart from the published Helm repository.',
      'Chart `appVersion` acts as the default version source for first-party runtime images.'
    ]
  },
  {
    id: 'configuration',
    title: 'Configuration',
    points: [
      'Use Helm values to configure ingress, public URLs, persistence, image tags, and component-level settings.',
      'Environment-specific hostnames and routing belong in your own values files, not in the public site.',
      'The stack is designed so public-facing installs can override defaults without patching templates.'
    ]
  },
  {
    id: 'development',
    title: 'Development workflow',
    points: [
      'Local development can use Tilt and the repo-local charts, but that path is intentionally separate from public install docs.',
      'Backend services are implemented in Go, the site and Hive UI are implemented in Vue, and packaging is handled through Helm.',
      'CI validates charts, builds apps, and exercises the packaged install path before publication.'
    ]
  }
]
</script>

<template>
  <div class="min-h-screen bg-background pb-20 text-on-background md:pb-0">
    <main class="mx-auto max-w-7xl px-6 pt-20 pb-16 md:py-24 md:pt-24">
      <section class="grid gap-8 border border-outline-variant/20 bg-surface-container-low p-8 shadow-[0_0_30px_rgba(0,255,65,0.06)] lg:grid-cols-[minmax(0,1.35fr)_minmax(320px,0.85fr)]">
        <div>
          <div class="mb-5 inline-flex items-center gap-2 border border-primary-container/30 bg-primary-container/5 px-3 py-1 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">
            Documentation Node // Online
          </div>
          <h1 class="max-w-4xl font-headline text-4xl font-black tracking-tighter uppercase md:text-6xl">
            Public documentation for installing and understanding Vinculum.
          </h1>
          <p class="mt-6 max-w-3xl text-base leading-8 text-on-surface-variant md:text-lg">
            This page focuses on the shipped platform: what the Helm chart installs, how the main runtime pieces fit together, and where to go next for release and repository details.
          </p>
          <div class="mt-8 flex flex-wrap gap-4">
            <NuxtLink to="/" class="bg-primary-container px-6 py-3 font-headline text-sm font-bold tracking-[0.2em] text-on-primary uppercase transition-all hover:shadow-[0_0_18px_rgba(0,255,65,0.35)]">
              Return Home
            </NuxtLink>
            <a href="https://github.com/florianwenzel/vinculum" class="border border-outline px-6 py-3 font-headline text-sm font-bold tracking-[0.2em] text-secondary uppercase transition-all hover:bg-surface-variant">
              Open Repository
            </a>
          </div>
        </div>

        <div class="border border-outline-variant/30 bg-surface-container-lowest p-5 font-mono text-sm text-secondary/80">
          <div class="mb-4 flex items-center justify-between border-b border-outline-variant/20 pb-2 text-[10px] uppercase tracking-[0.3em] text-primary-container">
            <span>Install Sequence</span>
            <span>HELM_PATH</span>
          </div>
          <div class="space-y-4">
            <div v-for="step in installSteps" :key="step" class="flex gap-3">
              <span class="text-outline">$</span>
              <span class="break-all text-on-surface">{{ step }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="mt-12 grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
        <div class="border border-outline-variant/20 bg-surface-container p-8">
          <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">Install Notes</div>
          <h2 class="font-headline text-3xl font-black tracking-tight uppercase">What the chart gives you</h2>
          <ul class="mt-6 space-y-4 text-sm leading-7 text-on-surface-variant md:text-base">
            <li v-for="note in installNotes" :key="note" class="border-l-2 border-primary-container/50 pl-4">
              {{ note }}
            </li>
          </ul>
        </div>

        <div class="border border-outline-variant/20 bg-surface-container-low p-8">
          <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">Core API Resources</div>
          <h2 class="font-headline text-3xl font-black tracking-tight uppercase">First resources to know</h2>
          <div class="mt-6 space-y-3 font-mono text-sm text-secondary/85">
            <div v-for="resource in apiResources" :key="resource" class="border border-outline-variant/20 bg-surface-container-lowest px-4 py-3">
              {{ resource }}
            </div>
          </div>
        </div>
      </section>

      <section class="mt-12 grid gap-4 lg:grid-cols-[minmax(0,1.05fr)_minmax(320px,0.95fr)]">
        <div class="border border-outline-variant/20 bg-surface-container-low p-8">
          <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">Published Artifacts</div>
          <h2 class="font-headline text-3xl font-black tracking-tight uppercase">Where releases live</h2>
          <div class="mt-6 space-y-3 font-mono text-sm text-secondary/85">
            <div v-for="artifact in publishedArtifacts" :key="artifact" class="border border-outline-variant/20 bg-surface-container-lowest px-4 py-3 break-all">
              {{ artifact }}
            </div>
          </div>
        </div>

        <div class="border border-outline-variant/20 bg-surface-container p-8">
          <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">Public Scope</div>
          <h2 class="font-headline text-3xl font-black tracking-tight uppercase">What this documentation covers</h2>
          <ul class="mt-6 space-y-4 text-sm leading-7 text-on-surface-variant md:text-base">
            <li class="border-l-2 border-primary-container/50 pl-4">Install the published platform from Helm without relying on repo-local development tooling.</li>
            <li class="border-l-2 border-primary-container/50 pl-4">Understand the main runtime components and the core resources exposed by the API.</li>
            <li class="border-l-2 border-primary-container/50 pl-4">Find the canonical source docs for release flow, chart publishing, and implementation details in the repository.</li>
          </ul>
        </div>
      </section>

      <section class="mt-12">
        <div class="mb-6 flex items-end justify-between gap-4">
          <div>
            <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">Architecture</div>
            <h2 class="font-headline text-3xl font-black tracking-tight uppercase md:text-4xl">Main runtime pieces</h2>
          </div>
          <div class="font-label text-[10px] tracking-[0.3em] text-outline uppercase">Platform Overview</div>
        </div>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <article v-for="card in architectureCards" :key="card.title" class="border border-outline-variant/20 bg-surface-container p-6">
            <h3 class="font-headline text-2xl font-bold uppercase">{{ card.title }}</h3>
            <p class="mt-3 text-sm leading-7 text-on-surface-variant">{{ card.text }}</p>
          </article>
        </div>
      </section>

      <section class="mt-12 border border-outline-variant/20 bg-surface-container-low p-8">
        <div class="mb-8 text-center">
          <div class="mb-3 font-label text-[10px] tracking-[0.3em] text-primary-container uppercase">Workflow</div>
          <h2 class="font-headline text-3xl font-black tracking-tight uppercase md:text-4xl">How work moves through the platform</h2>
        </div>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <article v-for="step in workflowSteps" :key="step.step" class="border border-outline-variant/20 bg-surface-container p-6">
            <div class="mb-4 flex h-12 w-12 items-center justify-center border border-primary-container bg-surface-container-lowest font-headline text-lg font-black text-primary-container">
              {{ step.step }}
            </div>
            <h3 class="font-headline text-xl font-bold uppercase">{{ step.title }}</h3>
            <p class="mt-3 text-sm leading-7 text-on-surface-variant">{{ step.text }}</p>
          </article>
        </div>
      </section>

      <section class="mt-12 space-y-6">
        <SiteDocsDocsSectionBlock
          v-for="section in docsSections"
          :id="section.id"
          :key="section.id"
          :title="section.title"
          :points="section.points"
        />
      </section>
    </main>
  </div>
</template>
