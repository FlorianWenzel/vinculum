<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Dropdown from 'primevue/dropdown'
import InputText from 'primevue/inputtext'
import Menubar from 'primevue/menubar'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import GlassCard from './components/GlassCard.vue'

const sections = [
  { id: 'projects', label: 'Projects' },
  { id: 'requirements', label: 'Requirements' },
  { id: 'tasks', label: 'Tasks' },
  { id: 'drones', label: 'Drones' },
  { id: 'links', label: 'Links' },
]

const activeSection = ref('projects')
const loading = ref(true)
const error = ref('')
const notice = ref('')
const refreshHandle = ref(null)
const state = ref({ repositories: [], requirements: [], tasks: [], drones: [], accesses: [], reviews: [], jobs: [] })

const projectForm = ref({ name: 'new-project', description: '', private: true })
const requirementForm = ref({
  title: 'New requirement',
  repositoryRef: '',
  status: 'TODO',
  body: '# Context\n\nDescribe the feature.\n\n# Acceptance Criteria\n\n- Add acceptance criteria',
  dependsOn: '',
})
const taskForm = ref({
  name: 'new-task',
  repositoryRef: '',
  requirementRef: '',
  droneRef: '',
  prompt: 'Implement the requested change in the repository, keep the diff focused, and leave the branch ready for review.',
})
const droneForm = ref({
  name: 'new-drone',
  role: 'coder',
  forgejoUsername: 'new-drone-bot',
  model: 'opencode/minimax-m2.5-free',
  instructions: 'You are a Vinculum drone. Keep changes small and validate carefully.',
  providerAuth: '{"opencode":{"type":"api","key":"replace-me"}}',
})
const linkForm = ref({ name: 'grant-one', droneRef: '', repositoryRef: '', permission: 'write' })

const projectOptions = computed(() => state.value.repositories
  .map((item) => ({ label: item.metadata?.name, value: item.metadata?.name }))
  .sort((a, b) => a.label.localeCompare(b.label)))

const droneOptions = computed(() => state.value.drones
  .map((item) => ({ label: item.metadata?.name, value: item.metadata?.name }))
  .sort((a, b) => a.label.localeCompare(b.label)))

const requirementOptions = computed(() => state.value.requirements
  .map((item) => ({
    label: item.status?.observedTitle || item.metadata?.name,
    value: item.metadata?.name,
  }))
  .sort((a, b) => a.label.localeCompare(b.label)))

const projects = computed(() => [...state.value.repositories].sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || '')))
const requirements = computed(() => [...state.value.requirements].sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || '')))
const tasks = computed(() => [...state.value.tasks].sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || '')))
const drones = computed(() => [...state.value.drones].sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || '')))
const links = computed(() => [...state.value.accesses].sort((a, b) => (a.metadata?.name || '').localeCompare(b.metadata?.name || '')))

const stats = computed(() => [
  { label: 'Projects', value: projects.value.length },
  { label: 'Requirements', value: requirements.value.length },
  { label: 'Tasks', value: tasks.value.length },
  { label: 'Drones', value: drones.value.length },
  { label: 'Links', value: links.value.length },
  { label: 'Busy drones', value: drones.value.filter((item) => (item.status?.activeTasks ?? 0) > 0).length },
])

const menuItems = computed(() => sections.map((item) => ({
  label: item.label,
  command: () => { activeSection.value = item.id },
  class: activeSection.value === item.id ? 'menu-item-active' : 'menu-item-inactive',
})))

const selectedProject = computed(() => projects.value.find((item) => item.metadata?.name === linkForm.value.repositoryRef) || projects.value[0] || null)
const selectedDrone = computed(() => drones.value.find((item) => item.metadata?.name === linkForm.value.droneRef) || drones.value[0] || null)

function projectLinks(projectName) {
  return links.value.filter((item) => item.spec?.repositoryRef === projectName)
}

function droneLinks(droneName) {
  return links.value.filter((item) => item.spec?.droneRef === droneName)
}

function setProject(name) {
  linkForm.value.repositoryRef = name
  activeSection.value = 'links'
}

function setDrone(name) {
  linkForm.value.droneRef = name
  activeSection.value = 'links'
}

function resetNotice() {
  error.value = ''
  notice.value = ''
}

async function load() {
  try {
    const res = await fetch('/api/overview')
    const json = await res.json()
    state.value = json
    error.value = ''
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function post(path, payload) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const text = await res.text()
  if (!res.ok) throw new Error(text || `Request failed: ${res.status}`)
  return text ? JSON.parse(text) : {}
}

async function createProject() {
  resetNotice()
  try {
    await post('/api/repositories', { ...projectForm.value, owner: 'vinculum' })
    notice.value = 'Project created.'
    projectForm.value.name = 'new-project'
    projectForm.value.description = ''
    await load()
  } catch (err) {
    error.value = err.message
  }
}

async function createDrone() {
  resetNotice()
  try {
    await post('/api/drones', droneForm.value)
    notice.value = 'Drone created.'
    droneForm.value.name = 'new-drone'
    droneForm.value.forgejoUsername = 'new-drone-bot'
    await load()
  } catch (err) {
    error.value = err.message
  }
}

async function createRequirement() {
  resetNotice()
  try {
    const dependsOn = requirementForm.value.dependsOn
      .split(/\n|,/)
      .map((item) => item.trim())
      .filter(Boolean)
    await post('/api/requirements', {
      title: requirementForm.value.title,
      repositoryRef: requirementForm.value.repositoryRef,
      status: requirementForm.value.status,
      body: requirementForm.value.body,
      dependsOn,
    })
    notice.value = 'Requirement created.'
    requirementForm.value.title = 'New requirement'
    requirementForm.value.status = 'TODO'
    requirementForm.value.body = '# Context\n\nDescribe the feature.\n\n# Acceptance Criteria\n\n- Add acceptance criteria'
    requirementForm.value.dependsOn = ''
    await load()
  } catch (err) {
    error.value = err.message
  }
}

async function createTask() {
  resetNotice()
  try {
    await post('/api/tasks', {
      name: taskForm.value.name,
      repositoryRef: taskForm.value.repositoryRef,
      requirementRef: taskForm.value.requirementRef || undefined,
      droneRef: taskForm.value.droneRef || undefined,
      prompt: taskForm.value.prompt,
    })
    notice.value = 'Task created.'
    taskForm.value.name = 'new-task'
    taskForm.value.requirementRef = ''
    taskForm.value.droneRef = ''
    taskForm.value.prompt = 'Implement the requested change in the repository, keep the diff focused, and leave the branch ready for review.'
    await load()
  } catch (err) {
    error.value = err.message
  }
}

async function createLink() {
  resetNotice()
  try {
    await post('/api/accesses', linkForm.value)
    notice.value = 'Link created.'
    linkForm.value.name = 'grant-one'
    await load()
  } catch (err) {
    error.value = err.message
  }
}

onMounted(() => {
  load()
  refreshHandle.value = setInterval(load, 10000)
})

onBeforeUnmount(() => {
  if (refreshHandle.value) clearInterval(refreshHandle.value)
})
</script>

<template>
  <main class="hive-shell px-4 py-4 sm:px-6">
    <div class="mx-auto flex w-full max-w-4xl flex-col gap-4">
      <Menubar :model="menuItems" class="nav-bar" data-testid="nav-row">
        <template #start>
          <div class="brand-block">
            <h1 class="text-xl font-black text-slate-50">Vinculum</h1>
          </div>
        </template>
      </Menubar>

      <section class="summary-grid">
        <GlassCard v-for="stat in stats" :key="stat.label" class="dark-card">
          <template #title>{{ stat.label }}</template>
          <strong>{{ stat.value }}</strong>
        </GlassCard>
      </section>

      <p v-if="error" class="rounded-2xl border border-red-500/20 bg-red-950/40 px-4 py-3 text-red-200">{{ error }}</p>
      <p v-if="notice" class="rounded-2xl border border-emerald-500/20 bg-emerald-950/40 px-4 py-3 text-emerald-200">{{ notice }}</p>

      <section v-if="activeSection === 'projects'" class="section-grid">
        <GlassCard>
          <template #title>Create project</template>
          <div class="toolbar">
            <label class="text-sm text-slate-300">Name<InputText v-model="projectForm.name" class="soft-input" data-testid="project-name-input" /></label>
            <label class="text-sm text-slate-300">Description<Textarea v-model="projectForm.description" rows="4" class="soft-input" data-testid="project-description-input" /></label>
            <Button type="button" label="Create project" class="w-full" data-testid="create-project-button" @click="createProject" />
          </div>
        </GlassCard>

        <GlassCard>
          <template #title>Projects</template>
          <div class="section-grid">
              <article v-for="project in projects" :key="project.metadata.uid" class="item-card" :data-testid="`project-card-${project.metadata.name}`">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <h3 class="text-lg font-semibold text-slate-50">{{ project.metadata.name }}</h3>
                    <p class="text-sm text-slate-400">{{ project.spec.owner }}</p>
                  </div>
                  <Tag :value="project.status?.phase || 'Ready'" severity="success" />
                </div>
                <p class="mt-3 text-sm leading-6 text-slate-300">{{ project.spec.description || 'No description yet.' }}</p>
                <div class="chip-row mt-4">
                  <Tag v-for="link in projectLinks(project.metadata.name).slice(0, 4)" :key="link.metadata.uid" :value="link.spec.droneRef" severity="info" />
                  <Tag v-if="!projectLinks(project.metadata.name).length" value="No drones linked" severity="secondary" />
                </div>
                <div class="mt-4 flex flex-wrap gap-2">
                  <Button type="button" :data-testid="`project-link-${project.metadata.name}`" label="Link drone" severity="success" outlined @click="setProject(project.metadata.name)" />
                  <Button type="button" label="Select" severity="secondary" outlined @click="linkForm.repositoryRef = project.metadata.name; activeSection = 'links'" />
                </div>
              </article>
            </div>
        </GlassCard>
      </section>

      <section v-else-if="activeSection === 'requirements'" class="section-grid">
        <GlassCard>
          <template #title>Create requirement</template>
          <div class="toolbar">
            <label class="text-sm text-slate-300">Repository<Dropdown v-model="requirementForm.repositoryRef" :options="projectOptions" option-label="label" option-value="value" class="soft-input" data-testid="requirement-project-select" /></label>
            <label class="text-sm text-slate-300">Title<InputText v-model="requirementForm.title" class="soft-input" data-testid="requirement-title-input" /></label>
            <label class="text-sm text-slate-300">Status<Dropdown v-model="requirementForm.status" :options="['TODO', 'IN_PROGRESS', 'WAITING', 'IN_REVIEW', 'DONE', 'BLOCKED']" class="soft-input" data-testid="requirement-status-select" /></label>
            <label class="text-sm text-slate-300">Depends on<Textarea v-model="requirementForm.dependsOn" rows="3" class="soft-input" data-testid="requirement-depends-input" /></label>
            <label class="text-sm text-slate-300">Body<Textarea v-model="requirementForm.body" rows="8" class="soft-input" data-testid="requirement-body-input" /></label>
            <Button type="button" label="Create requirement" class="w-full" data-testid="create-requirement-button" @click="createRequirement" />
          </div>
        </GlassCard>

        <GlassCard>
          <template #title>Requirements</template>
          <div class="section-grid">
            <article v-for="requirement in requirements" :key="requirement.metadata.uid" class="item-card" :data-testid="`requirement-card-${requirement.metadata.name}`">
              <div class="flex items-start justify-between gap-3">
                <div>
                  <h3 class="text-lg font-semibold text-slate-50">{{ requirement.status?.observedTitle || requirement.metadata.name }}</h3>
                  <p class="text-sm text-slate-400">{{ requirement.spec.repositoryRef }} · {{ requirement.spec.filePath }}</p>
                </div>
                <Tag :value="requirement.status?.observedStatus || requirement.status?.phase || 'TODO'" severity="info" />
              </div>
              <p class="mt-3 text-sm leading-6 text-slate-300">Branch: {{ requirement.status?.observedBranch || 'pending' }}</p>
              <div class="chip-row mt-4">
                <Tag v-for="dep in requirement.status?.observedDependsOn || []" :key="dep" :value="dep" severity="secondary" />
                <Tag v-if="!(requirement.status?.observedDependsOn || []).length" value="No dependencies" severity="secondary" />
              </div>
            </article>

            <article v-if="!requirements.length" class="item-card">
              <p class="text-slate-300">No requirements created yet.</p>
            </article>
          </div>
        </GlassCard>
      </section>

      <section v-else-if="activeSection === 'tasks'" class="section-grid">
        <GlassCard>
          <template #title>Create task</template>
          <div class="toolbar">
            <label class="text-sm text-slate-300">Name<InputText v-model="taskForm.name" class="soft-input" data-testid="task-name-input" /></label>
            <label class="text-sm text-slate-300">Repository<Dropdown v-model="taskForm.repositoryRef" :options="projectOptions" option-label="label" option-value="value" class="soft-input" data-testid="task-project-select" /></label>
            <label class="text-sm text-slate-300">Requirement<Dropdown v-model="taskForm.requirementRef" :options="requirementOptions" option-label="label" option-value="value" placeholder="Optional" show-clear class="soft-input" data-testid="task-requirement-select" /></label>
            <label class="text-sm text-slate-300">Assigned drone<Dropdown v-model="taskForm.droneRef" :options="droneOptions" option-label="label" option-value="value" placeholder="Optional" show-clear class="soft-input" data-testid="task-drone-select" /></label>
            <label class="text-sm text-slate-300">Prompt<Textarea v-model="taskForm.prompt" rows="6" class="soft-input" data-testid="task-prompt-input" /></label>
            <Button type="button" label="Create task" class="w-full" data-testid="create-task-button" @click="createTask" />
          </div>
        </GlassCard>

        <GlassCard>
          <template #title>Tasks</template>
          <div class="section-grid">
            <article v-for="task in tasks" :key="task.metadata.uid" class="item-card" :data-testid="`task-card-${task.metadata.name}`">
              <div class="flex items-start justify-between gap-3">
                <div>
                  <h3 class="text-lg font-semibold text-slate-50">{{ task.metadata.name }}</h3>
                  <p class="text-sm text-slate-400">{{ task.spec.repositoryRef }}<span v-if="task.spec.requirementRef"> · {{ task.spec.requirementRef }}</span></p>
                </div>
                <Tag :value="task.status?.phase || 'Planned'" :severity="task.status?.phase === 'Merged' ? 'success' : task.status?.phase === 'Failed' ? 'danger' : 'info'" />
              </div>
              <p class="mt-3 text-sm leading-6 text-slate-300">{{ task.status?.summary || task.spec.prompt }}</p>
              <div class="chip-row mt-4">
                <Tag :value="`Role: ${task.spec.role || 'coder'}`" severity="secondary" />
                <Tag :value="`Drone: ${task.spec.droneRef || task.status?.assignedDrone || 'unassigned'}`" severity="secondary" />
              </div>
              <p v-if="task.status?.pullRequestUrl" class="mt-4 break-all text-xs text-emerald-300">PR: {{ task.status.pullRequestUrl }}</p>
            </article>

            <article v-if="!tasks.length" class="item-card">
              <p class="text-slate-300">No tasks created yet.</p>
            </article>
          </div>
        </GlassCard>
      </section>

      <section v-else-if="activeSection === 'drones'" class="section-grid">
        <GlassCard>
          <template #title>Create drone</template>
          <div class="toolbar">
            <label class="text-sm text-slate-300">Name<InputText v-model="droneForm.name" class="soft-input" data-testid="drone-name-input" /></label>
            <label class="text-sm text-slate-300">Role<InputText v-model="droneForm.role" class="soft-input" data-testid="drone-role-input" /></label>
            <label class="text-sm text-slate-300">Forgejo username<InputText v-model="droneForm.forgejoUsername" class="soft-input" data-testid="drone-username-input" /></label>
            <label class="text-sm text-slate-300">Model<InputText v-model="droneForm.model" class="soft-input" data-testid="drone-model-input" /></label>
            <label class="text-sm text-slate-300">Instructions<Textarea v-model="droneForm.instructions" rows="4" class="soft-input" data-testid="drone-instructions-input" /></label>
            <label class="text-sm text-slate-300">Provider auth<Textarea v-model="droneForm.providerAuth" rows="4" class="soft-input" data-testid="drone-provider-auth-input" /></label>
            <Button type="button" label="Create drone" class="w-full" data-testid="create-drone-button" @click="createDrone" />
          </div>
        </GlassCard>

        <GlassCard>
          <template #title>Drones</template>
          <div class="section-grid">
              <article v-for="drone in drones" :key="drone.metadata.uid" class="item-card" :data-testid="`drone-card-${drone.metadata.name}`">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <h3 class="text-lg font-semibold text-slate-50">{{ drone.metadata.name }}</h3>
                    <p class="text-sm text-slate-400">{{ drone.spec.role }} · {{ drone.spec.forgejoUsername }}</p>
                  </div>
                  <Tag :value="(drone.status?.activeTasks ?? 0) > 0 ? 'Working' : 'Ready'" :severity="(drone.status?.activeTasks ?? 0) > 0 ? 'warn' : 'success'" />
                </div>
                <p class="mt-3 text-sm leading-6 text-slate-300">{{ drone.spec.model }}</p>
                <div class="chip-row mt-4">
                  <Tag v-for="link in droneLinks(drone.metadata.name).slice(0, 4)" :key="link.metadata.uid" :value="link.spec.repositoryRef" severity="info" />
                  <Tag v-if="!droneLinks(drone.metadata.name).length" value="No projects linked" severity="secondary" />
                </div>
                <div class="mt-4 flex flex-wrap gap-2">
                  <Button type="button" :data-testid="`drone-link-${drone.metadata.name}`" label="Link project" severity="success" outlined @click="setDrone(drone.metadata.name)" />
                  <Button type="button" label="Select" severity="secondary" outlined @click="linkForm.droneRef = drone.metadata.name; activeSection = 'links'" />
                </div>
              </article>
            </div>
        </GlassCard>
      </section>

      <section v-else class="section-grid">
        <GlassCard>
          <template #title>Create link</template>
          <div class="toolbar">
            <label class="text-sm text-slate-300">Name<InputText v-model="linkForm.name" class="soft-input" data-testid="link-name-input" /></label>
            <label class="text-sm text-slate-300">Project<Dropdown v-model="linkForm.repositoryRef" :options="projectOptions" option-label="label" option-value="value" class="soft-input" data-testid="link-project-select" /></label>
            <label class="text-sm text-slate-300">Drone<Dropdown v-model="linkForm.droneRef" :options="droneOptions" option-label="label" option-value="value" class="soft-input" data-testid="link-drone-select" /></label>
            <label class="text-sm text-slate-300">Permission<Dropdown v-model="linkForm.permission" :options="['read', 'write', 'admin']" class="soft-input" data-testid="link-permission-select" /></label>
            <Button type="button" label="Create link" class="w-full" data-testid="create-link-button" @click="createLink" />
          </div>
        </GlassCard>

        <GlassCard>
          <template #title>Current links</template>
          <div class="section-grid">
              <article v-for="link in links" :key="link.metadata.uid" class="item-card">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <h3 class="text-lg font-semibold text-slate-50">{{ link.metadata.name }}</h3>
                    <p class="text-sm text-slate-400">{{ link.spec.repositoryRef }} → {{ link.spec.droneRef }}</p>
                  </div>
                  <Tag :value="link.spec.permission" severity="info" />
                </div>
                <p class="mt-3 text-sm leading-6 text-slate-300">{{ link.status?.phase || 'Ready' }}</p>
              </article>

              <article v-if="!links.length" class="item-card">
                <p class="text-slate-300">No links created yet.</p>
              </article>
            </div>
        </GlassCard>

        <GlassCard v-if="selectedProject || selectedDrone">
          <template #title>Selection</template>
          <div class="section-grid">
            <div v-if="selectedProject">
              <p class="muted text-xs uppercase tracking-[0.2em]">Project</p>
              <p class="text-slate-50">{{ selectedProject.metadata.name }}</p>
            </div>
            <div v-if="selectedDrone">
              <p class="muted text-xs uppercase tracking-[0.2em]">Drone</p>
              <p class="text-slate-50">{{ selectedDrone.metadata.name }}</p>
            </div>
          </div>
        </GlassCard>
      </section>
    </div>
  </main>
</template>
