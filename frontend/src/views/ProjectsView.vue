<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { useProjectsStore } from '../stores/projects'
import { useToastStore } from '../stores/toasts'
import type { Project, ProjectRole } from '../domain/types'

const projects = useProjectsStore()
const toasts = useToastStore()
const router = useRouter()
const showCreate = ref(false)
const activeProject = ref<Project | null>(null)
const busy = ref(false)
const creatingBoardFor = ref('')
const memberFor = ref('')
const projectForm = reactive({ name: '', description: '' })
const boardForm = reactive({ name: '', description: '' })
const memberForm = reactive({ user_id: '', role: 'member' as ProjectRole })

onMounted(() => { void projects.load() })

async function createProject() {
  if (!projectForm.name.trim()) return
  busy.value = true
  try {
    const project = await projects.create({ name: projectForm.name.trim(), description: projectForm.description.trim() || undefined })
    projectForm.name = ''; projectForm.description = ''; showCreate.value = false
    toasts.show('Project created', { tone: 'success', message: `${project.name} is ready for a board.` })
  } catch (error) { toasts.show('Could not create project', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { busy.value = false }
}

async function openProject(project: Project) {
  activeProject.value = project
  try {
    await projects.loadBoards(project.id)
  } catch (error) { toasts.show('Could not load boards', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
}

async function createBoard(projectId: string) {
  if (!boardForm.name.trim()) return
  busy.value = true
  try {
    const board = await projects.createBoard(projectId, { name: boardForm.name.trim(), description: boardForm.description.trim() || undefined })
    boardForm.name = ''; boardForm.description = ''; creatingBoardFor.value = ''
    toasts.show('Board created', { tone: 'success' })
    await router.push(`/projects/${projectId}/boards/${board.id}`)
  } catch (error) { toasts.show('Could not create board', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { busy.value = false }
}

async function addMember(projectId: string) {
  if (!memberForm.user_id.trim()) return
  busy.value = true
  try {
    await projects.addMember(projectId, memberForm.user_id.trim(), memberForm.role)
    toasts.show('Member added', { tone: 'success' }); memberForm.user_id = ''; memberFor.value = ''
  } catch (error) { toasts.show('Could not add member', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { busy.value = false }
}

function toggleBoards(project: Project) {
  if (activeProject.value?.id === project.id) activeProject.value = null
  else void openProject(project)
}
function canAdmin(project: Project) { return project.role === 'admin' }
</script>

<template>
  <main class="page page--projects">
    <section class="page-heading">
      <div><p class="eyebrow">Your workspace</p><h1>Projects</h1><p>Everything your team is moving forward.</p></div>
      <button class="button button--primary" data-testid="new-project" @click="showCreate = true"><AppIcon name="plus" :size="18" /> New project</button>
    </section>

    <div v-if="projects.error" class="alert alert--error"><span>{{ projects.error }}</span><button class="text-button" @click="projects.load()">Try again</button></div>
    <section v-if="projects.loading" class="project-grid" aria-label="Loading projects">
      <div v-for="i in 3" :key="i" class="project-card skeleton"></div>
    </section>
    <section v-else-if="projects.projects.length" class="project-grid" data-testid="project-list">
      <article v-for="(project, index) in projects.projects" :key="project.id" class="project-card" :class="{ 'project-card--open': activeProject?.id === project.id }" :data-testid="`project-${project.id}`">
        <button class="project-card__main" @click="toggleBoards(project)">
          <span class="project-card__symbol" :style="{ '--hue': `${(index * 61 + 225) % 360}` }">{{ project.name.charAt(0).toUpperCase() }}</span>
          <span class="project-card__copy"><strong>{{ project.name }}</strong><small>{{ project.description || 'A focused place for this team’s work.' }}</small></span>
          <AppIcon class="project-card__arrow" name="arrow" :size="20" />
        </button>
        <div class="project-card__meta">
          <span class="role-pill">{{ project.role || (project.owner_id ? 'member' : 'admin') }}</span>
          <span>Team access</span>
        </div>
        <Transition name="expand">
          <div v-if="activeProject?.id === project.id" class="project-boards">
            <div class="section-row"><span>Boards</span><div v-if="canAdmin(project)"><button class="text-button" @click.stop="memberFor = project.id">+ Member</button><button class="text-button" @click.stop="creatingBoardFor = project.id">+ Board</button></div></div>
            <button v-for="board in projects.boardsByProject[project.id] || []" :key="board.id" class="board-row" @click.stop="router.push(`/projects/${project.id}/boards/${board.id}`)"><span class="board-row__dot"></span><span>{{ board.name }}</span><AppIcon name="arrow" :size="17" /></button>
            <p v-if="projects.boardsByProject[project.id]?.length === 0" class="empty-inline">No boards yet. Create the first one.</p>
          </div>
        </Transition>
      </article>
    </section>
    <section v-else class="empty-state">
      <span class="empty-state__art"><AppIcon name="grid" :size="34" /></span>
      <h2>Your next project starts here</h2><p>Create a project, invite your team, and make the work visible.</p>
      <button class="button button--primary" @click="showCreate = true"><AppIcon name="plus" /> Create a project</button>
    </section>

    <div v-if="showCreate" class="modal-backdrop" @mousedown.self="showCreate = false">
      <form class="modal modal--compact" data-testid="project-modal" @submit.prevent="createProject">
        <div class="modal__head"><div><p class="eyebrow">New workspace</p><h2>Create a project</h2></div><button type="button" class="icon-button" aria-label="Close" @click="showCreate = false"><AppIcon name="close" /></button></div>
        <label class="field"><span>Project name</span><input v-model="projectForm.name" autofocus required maxlength="100" placeholder="e.g. Product launch" data-testid="project-name" /></label>
        <label class="field"><span>Description <small>optional</small></span><textarea v-model="projectForm.description" rows="3" maxlength="500" placeholder="What is this project about?"></textarea></label>
        <div class="modal__actions"><button type="button" class="button button--ghost" @click="showCreate = false">Cancel</button><button class="button button--primary" :disabled="busy || !projectForm.name.trim()">{{ busy ? 'Creating…' : 'Create project' }}</button></div>
      </form>
    </div>

    <div v-if="creatingBoardFor" class="modal-backdrop" @mousedown.self="creatingBoardFor = ''">
      <form class="modal modal--compact" data-testid="board-modal" @submit.prevent="createBoard(creatingBoardFor)">
        <div class="modal__head"><div><p class="eyebrow">New board</p><h2>Set up a board</h2></div><button type="button" class="icon-button" aria-label="Close" @click="creatingBoardFor = ''"><AppIcon name="close" /></button></div>
        <label class="field"><span>Board name</span><input v-model="boardForm.name" autofocus required maxlength="100" placeholder="e.g. Main board" data-testid="board-name" /></label>
        <label class="field"><span>Description <small>optional</small></span><textarea v-model="boardForm.description" rows="3"></textarea></label>
        <div class="modal__actions"><button type="button" class="button button--ghost" @click="creatingBoardFor = ''">Cancel</button><button class="button button--primary" :disabled="busy || !boardForm.name.trim()">Create board</button></div>
      </form>
    </div>

    <div v-if="memberFor" class="modal-backdrop" @mousedown.self="memberFor = ''">
      <form class="modal modal--compact" data-testid="member-modal" @submit.prevent="addMember(memberFor)">
        <div class="modal__head"><div><p class="eyebrow">Project access</p><h2>Add a member</h2></div><button type="button" class="icon-button" aria-label="Close" @click="memberFor = ''"><AppIcon name="close" /></button></div>
        <label class="field"><span>User ID</span><input v-model="memberForm.user_id" required placeholder="Keycloak user UUID" data-testid="member-user-id" /></label>
        <label class="field"><span>Role</span><select v-model="memberForm.role"><option value="member">Member</option><option value="viewer">Viewer</option><option value="admin">Admin</option></select></label>
        <div class="modal__actions"><button type="button" class="button button--ghost" @click="memberFor = ''">Cancel</button><button class="button button--primary" :disabled="busy">Add member</button></div>
      </form>
    </div>
  </main>
</template>
