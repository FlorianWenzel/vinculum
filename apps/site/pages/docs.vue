<script setup lang="ts">
const installCommand = `helm install vinculum oci://ghcr.io/florianwenzel/helm/vinculum \\
  --version 0.2.0 \\
  -n vinculum-system \\
  --create-namespace`

const sections = [
  {
    id: 'architecture',
    title: 'Architecture',
    points: [
      'The umbrella chart deploys infrastructure, Vinculum Infra, the orchestrator, and the Hive UI into one namespace.',
      'Forgejo handles repositories, Keycloak handles identity, and PostgreSQL backs both platform services.',
      'The orchestrator introduces CRDs for drones, repositories, requirements, tasks, reviews, and access grants.'
    ]
  },
  {
    id: 'deployment',
    title: 'Deployment',
    points: [
      'Charts are packaged from helm/ and published both as OCI artifacts in GHCR and as a classic Helm repository on GitHub Pages.',
      'First-party image tags default to the umbrella chart appVersion, so a chart release also defines the default application image set.',
      'GitHub Actions validates, packages, and publishes images and charts from main.'
    ]
  },
  {
    id: 'operations',
    title: 'Operations',
    points: [
      'Public routing on zora terminates through Cloudflare Tunnel and forwards to the Istio ingress gateway.',
      'Root domain vincula.dev serves this site, while app.vincula.dev, git.vincula.dev, and id.vincula.dev expose the control surfaces.',
      'A fresh reinstall should recreate pods, PVCs, OIDC wiring, and public ingress without manual data recovery.'
    ]
  }
]
</script>

<template>
  <div class="site-shell site-shell--docs">
    <section class="hero hero--docs">
      <div class="hero__nav">
        <div>
          <p class="eyebrow">Documentation</p>
          <p class="hero__tag">Practical notes for operators and agents</p>
        </div>
        <nav>
          <a href="/">Home</a>
          <a href="https://github.com/florianwenzel/vinculum/blob/main/Deployment.md">Deployment.md</a>
        </nav>
      </div>
      <div class="hero__content hero__content--docs">
        <div class="hero__copy">
          <h1>Deploy, inspect, and operate the Vinculum stack.</h1>
          <p class="lead">
            This site gives the short operational map. The repository remains the source of truth for chart values,
            workflows, and implementation details.
          </p>
        </div>
        <div class="hero__panel hero__panel--code">
          <span class="panel__title">OCI install</span>
          <pre>{{ installCommand }}</pre>
        </div>
      </div>
    </section>

    <section class="section section--tight">
      <div class="card-grid card-grid--three">
        <article class="card card--link">
          <h3>Deployment.md</h3>
          <p>Exact release and publishing flow for agents interacting with GHCR, charts, and workflows.</p>
          <a href="https://github.com/florianwenzel/vinculum/blob/main/Deployment.md">Open source doc</a>
        </article>
        <article class="card card--link">
          <h3>README</h3>
          <p>High-level architecture, install notes, and local development workflow.</p>
          <a href="https://github.com/florianwenzel/vinculum/blob/main/README.md">Open README</a>
        </article>
        <article class="card card--link">
          <h3>Runtime surfaces</h3>
          <p>Use the live stack: Hive UI, Forgejo, and Keycloak on the public vincula.dev subdomains.</p>
          <a href="https://app.vincula.dev">Open app.vincula.dev</a>
        </article>
      </div>
    </section>

    <section class="section">
      <div v-for="section in sections" :id="section.id" :key="section.id" class="docs-block">
        <p class="eyebrow">{{ section.title }}</p>
        <h2>{{ section.title }}</h2>
        <ul class="docs-list">
          <li v-for="point in section.points" :key="point">{{ point }}</li>
        </ul>
      </div>
    </section>
  </div>
</template>
