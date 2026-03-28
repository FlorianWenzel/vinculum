<script setup lang="ts">
const pillars = [
  {
    title: 'Autonomous Delivery Loops',
    text: 'Coder, reviewer, tester, and deployer drones work through requirements, implementation, review, and release as durable Kubernetes workloads.'
  },
  {
    title: 'Cluster-Native Control Plane',
    text: 'The orchestrator owns CRDs, schedules drone jobs, tracks task state, and exposes an API and Hive UI for operational visibility.'
  },
  {
    title: 'Built-In Platform Stack',
    text: 'Forgejo, Keycloak, PostgreSQL, bootstrap automation, and Helm packaging ship together so a clean cluster can become a working development platform quickly.'
  }
]

const surfaces = [
  { label: 'Hive UI', href: 'https://app.vincula.dev', note: 'Control surface for drones, repositories, tasks, and reviews.' },
  { label: 'Vinculum Code', href: 'https://git.vincula.dev', note: 'Forgejo-based code forge with OIDC and project automation.' },
  { label: 'Identity', href: 'https://id.vincula.dev', note: 'Keycloak realm used for browser auth and service integration.' }
]

const docs = [
  { title: 'Deployment Model', path: '/docs#deployment', text: 'Publishing flow, GHCR images, OCI Helm charts, and GitHub Actions behavior.' },
  { title: 'Operational Reset', path: '/docs#operations', text: 'Cluster reset, reinstall expectations, ingress, and Cloudflare tunnel placement.' },
  { title: 'Architecture Guide', path: '/docs#architecture', text: 'How the operator, infrastructure bootstrap, drone runtime, and UI fit together.' }
]
</script>

<template>
  <div class="site-shell">
    <section class="hero">
      <div class="hero__nav">
        <div>
          <p class="eyebrow">Vinculum</p>
          <p class="hero__tag">Autonomous software delivery on Kubernetes</p>
        </div>
        <nav>
          <a href="/docs">Docs</a>
          <a href="https://github.com/florianwenzel/vinculum">Source</a>
        </nav>
      </div>

      <div class="hero__content">
        <div class="hero__copy">
          <p class="kicker">Collective orchestration for real repositories</p>
          <h1>Turn requirements into reviewed, tested, and deployed software.</h1>
          <p class="lead">
            Vinculum combines drone workers, a Kubernetes operator, built-in identity,
            a Forgejo control surface, and deployable Helm charts into one cluster-native platform.
          </p>
          <div class="hero__actions">
            <a class="button button--primary" href="/docs">Read the docs</a>
            <a class="button button--ghost" href="https://git.vincula.dev">Open Vinculum Code</a>
          </div>
        </div>

        <div class="hero__panel">
          <span class="panel__title">Runtime surfaces</span>
          <ul>
            <li v-for="surface in surfaces" :key="surface.label">
              <a :href="surface.href">{{ surface.label }}</a>
              <span>{{ surface.note }}</span>
            </li>
          </ul>
        </div>
      </div>
    </section>

    <section class="section section--tight">
      <div class="section__header">
        <p class="eyebrow">Platform</p>
        <h2>Purpose-built for agent swarms, not single-shot demos.</h2>
      </div>
      <div class="card-grid card-grid--three">
        <article v-for="pillar in pillars" :key="pillar.title" class="card">
          <h3>{{ pillar.title }}</h3>
          <p>{{ pillar.text }}</p>
        </article>
      </div>
    </section>

    <section class="section">
      <div class="section__header">
        <p class="eyebrow">Workflow</p>
        <h2>One platform, four persistent layers.</h2>
      </div>
      <div class="timeline">
        <div class="timeline__item">
          <strong>1. Requirements and planning</strong>
          <p>Requirements become durable resources instead of chat history, so the operator can schedule, review, and revisit work later.</p>
        </div>
        <div class="timeline__item">
          <strong>2. Drone execution</strong>
          <p>Workers run in isolated pods with mounted instructions, auth, and repository access scoped through Kubernetes objects.</p>
        </div>
        <div class="timeline__item">
          <strong>3. Platform bootstrap</strong>
          <p>Vinculum Infra reconciles Keycloak and Forgejo so identity, organizations, and OIDC wiring come up with the stack.</p>
        </div>
        <div class="timeline__item">
          <strong>4. Ship and operate</strong>
          <p>Helm charts, GHCR images, and cluster ingress make the stack installable, reproducible, and externally reachable.</p>
        </div>
      </div>
    </section>

    <section class="section section--tight">
      <div class="section__header">
        <p class="eyebrow">Docs</p>
        <h2>Start with the operational model.</h2>
      </div>
      <div class="card-grid">
        <article v-for="item in docs" :key="item.title" class="card card--link">
          <h3>{{ item.title }}</h3>
          <p>{{ item.text }}</p>
          <NuxtLink :to="item.path">Open section</NuxtLink>
        </article>
      </div>
    </section>
  </div>
</template>
