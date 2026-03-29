<script setup lang="ts">
const desktopNav = [
  { label: 'ROLES', href: '#swarm', active: true },
  { label: 'ARCHITECTURE', href: '#architecture' },
  { label: 'DOCS', href: '/docs' }
]

const droneCards = [
  {
    title: 'Coder',
    unit: '0x01',
    icon: 'code_blocks',
    description:
      'Implements features, updates code paths, and writes the tests needed to move a task forward.',
    status: 'STATUS: IMPLEMENTING',
    fill: true,
    span: 'md:col-span-2'
  },
  {
    title: 'Reviewer',
    unit: '0x02',
    icon: 'fact_check',
    description:
      'Checks correctness, architecture, security, and change quality before work is approved.',
    fill: false,
    span: ''
  },
  {
    title: 'Tester',
    unit: '0x03',
    icon: 'biotech',
    description:
      'Runs validation steps and test workflows against new iterations and integration points.',
    fill: false,
    span: ''
  }
]

const workflowSteps = [
  {
    number: '01',
    title: 'Requirements',
    text: 'Capture product intent and acceptance criteria'
  },
  {
    number: '02',
    title: 'Planning',
    text: 'Break work into tasks and assign drones'
  },
  {
    number: '03',
    title: 'Execution',
    text: 'Run implementation, review, and test loops',
    active: true
  },
  {
    number: '04',
    title: 'Release',
    text: 'Promote validated changes toward deployment'
  }
]

const mobileDrones = [
  {
    title: 'CODER_DRONE',
    subtitle: 'Implementing changes',
    icon: 'code',
    accent: 'border-primary-container',
    iconColor: 'text-primary-container',
    subtitleColor: 'text-tertiary-fixed-dim',
    bars: ['bg-primary-container', 'bg-primary-container/30', 'bg-primary-container/30'],
    dimmed: false
  },
  {
    title: 'REVIEWER_UNIT',
    subtitle: 'Checking quality',
    icon: 'visibility',
    accent: 'border-primary-container',
    iconColor: 'text-primary-container',
    subtitleColor: 'text-tertiary-fixed-dim',
    bars: ['bg-primary-container', 'bg-primary-container', 'bg-primary-container/30'],
    dimmed: false
  },
  {
    title: 'TESTER_PROTOCOL',
    subtitle: 'Validation queue',
    icon: 'rule',
    accent: 'border-secondary-container',
    iconColor: 'text-secondary',
    subtitleColor: 'text-outline',
    bars: ['bg-outline/30', 'bg-outline/30', 'bg-outline/30'],
    dimmed: true
  },
  {
    title: 'DEPLOYER_CORE',
    subtitle: 'Preparing release',
    icon: 'rocket_launch',
    accent: 'border-primary-container',
    iconColor: 'text-primary-container',
    subtitleColor: 'text-tertiary-fixed-dim',
    bars: ['bg-primary-container', 'bg-primary-container', 'bg-primary-container'],
    dimmed: false,
    pulse: true
  }
]

const mobileWorkflow = [
  {
    number: '01',
    title: 'Requirements',
    text: 'Capture product intent, acceptance criteria, and constraints through the UI or API.'
  },
  {
    number: '02',
    title: 'Task Planning',
    text: 'Break requirements into technical tasks and assign drones with the right role and access.'
  },
  {
    number: '03',
    title: 'Execution',
    text: 'Run isolated drone workloads for coding, review, testing, and related automation loops.'
  },
  {
    number: '04',
    title: 'Review & Release',
    text: 'Use review decisions and deployment workflows to move validated changes toward production.'
  }
]

const stackItems = [
  { number: '01', title: 'Kubernetes Native', note: 'Orchestration' },
  { number: '02', title: 'Forgejo & Vinculum Code', note: 'Git Platform' },
  { number: '03', title: 'Keycloak Identity', note: 'Access Control' },
  { number: '04', title: 'Go / OpenCode', note: 'Core Engine' }
]

const mobileStack = [
  { icon: 'hub', label: 'Kubernetes' },
  { icon: 'account_tree', label: 'Forgejo' },
  { icon: 'database', label: 'PostgreSQL' },
  { icon: 'smart_toy', label: 'OpenCode' }
]

const footerLinks = [
  { label: 'DOCS', href: '/docs' },
  { label: 'DEPLOYMENT', href: '/docs#deployment' },
  { label: 'MIT LICENSE', href: 'https://github.com/florianwenzel/vinculum/blob/main/LICENSE' }
]
</script>

<template>
  <div class="min-h-screen bg-background text-on-background">
    <SiteHomeDesktopLanding
      :desktop-nav="desktopNav"
      :drone-cards="droneCards"
      :workflow-steps="workflowSteps"
      :stack-items="stackItems"
    />
    <SiteHomeSiteFooter :footer-links="footerLinks" class="hidden md:block" />
    <SiteHomeMobileLanding
      :mobile-drones="mobileDrones"
      :mobile-workflow="mobileWorkflow"
      :mobile-stack="mobileStack"
      :footer-links="footerLinks"
    />
  </div>
</template>
