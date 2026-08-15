<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import BoardSettings from '../components/BoardSettings.vue'
import TaskCard from '../components/TaskCard.vue'
import TaskModal from '../components/TaskModal.vue'
import type { BoardStatus, Task, WorkflowConditions, WorkflowRule } from '../domain/types'
import { useBoardStore } from '../stores/board'
import { useProjectsStore } from '../stores/projects'
import { useToastStore } from '../stores/toasts'

const route = useRoute()
const router = useRouter()
const boardStore = useBoardStore()
const projects = useProjectsStore()
const toasts = useToastStore()
const showSettings = ref(false)
const modalOpen = ref(false)
const selectedTask = ref<Task | null>(null)
const initialStatusId = ref('')
const saving = ref(false)
const settingsBusy = ref(false)
const draggingId = ref('')
const dropStatusId = ref('')
const modalKey = ref(0)

const projectId = computed(() => String(route.params.projectId || ''))
const boardId = computed(() => String(route.params.boardId || ''))
const project = computed(() => projects.projects.find((item) => item.id === projectId.value))
const boardOptions = computed(() => projects.boardsByProject[projectId.value] || [])
const canManageBoard = computed(() => project.value?.role === 'admin')
const canManageTasks = computed(() => project.value?.role === 'admin' || project.value?.role === 'member')
const members = computed(() => projects.membersByProject[projectId.value] || [])
const tasksByStatus = computed(() => {
  const result: Record<string, Task[]> = {}
  for (const status of boardStore.orderedStatuses) result[status.id] = []
  for (const task of boardStore.board?.tasks || []) (result[task.status_id] ||= []).push(task)
  return result
})

watch([projectId, boardId], async ([nextProjectId, nextBoardId]) => {
  if (!nextProjectId || !nextBoardId) return
  try {
    await Promise.all([
      boardStore.load(nextBoardId),
      projects.projects.length ? Promise.resolve() : projects.load(),
      projects.loadBoards(nextProjectId).catch(() => []),
    ])
  } catch (error) {
    toasts.show('Board unavailable', { tone: 'error', message: error instanceof Error ? error.message : undefined })
  }
}, { immediate: true })

function openNewTask(statusId = '') {
  if (!canManageTasks.value) return
  selectedTask.value = null
  initialStatusId.value = statusId || boardStore.orderedStatuses[0]?.id || ''
  modalOpen.value = true
  if (projectId.value) projects.loadMembers(projectId.value).catch(() => [])
}
function openTask(task: Task) {
  selectedTask.value = task
  initialStatusId.value = task.status_id
  modalOpen.value = true
  if (projectId.value) projects.loadMembers(projectId.value).catch(() => [])
}

async function saveTask(input: { title: string; description?: string; status_id: string; assignee_id?: string | null; deadline?: string | null; comment?: string }) {
  saving.value = true
  const existing = selectedTask.value
  const statusChanged = Boolean(existing && existing.status_id !== input.status_id)
  try {
    if (!existing) {
      const { comment: _comment, ...createInput } = input
      await boardStore.createTask(createInput)
      toasts.show('Task created', { tone: 'success', message: input.title })
      modalOpen.value = false
      return
    }

    const result = await boardStore.saveTaskEdits(existing.id, {
      title: input.title,
      description: input.description,
      assignee_id: input.assignee_id,
      deadline: input.deadline,
      comment: input.comment,
      statusChanged,
      targetStatusId: input.status_id,
    })
    // boardStore.updateTask() (called inside saveTaskEdits) replaces the task object in
    // board.tasks (rather than mutating it in place), so re-point selectedTask at that
    // fresh object. This keeps the modal's :task prop in sync with the just-saved
    // fields, and makes a reverted status (on a blocked transition, below) visible on
    // the forced remount.
    selectedTask.value = boardStore.board?.tasks.find((item) => item.id === existing.id) || existing

    if (result.status === 'blocked') {
      // Fields and comment are already persisted; only the status change was rejected.
      // Remount the modal so it re-reads the (now-reverted) status from the task in the store.
      modalKey.value += 1
      toasts.show('Status change blocked', {
        tone: 'error',
        message: result.reason ? `Fields and comment were saved. Status change was blocked: ${result.reason}` : 'Fields and comment were saved. The status change was blocked.',
        timeout: 6500,
      })
      return
    }

    toasts.show('Task saved', { tone: 'success', message: input.title })
    modalOpen.value = false
  } catch (error) {
    toasts.show('Could not save task', { tone: 'error', message: error instanceof Error ? error.message : undefined, timeout: 6500 })
  } finally { saving.value = false }
}

function startDrag(event: DragEvent, task: Task) {
  if (!canManageTasks.value) return
  draggingId.value = task.id
  event.dataTransfer?.setData('text/task-id', task.id)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}
async function dropTask(event: DragEvent, statusId: string) {
  event.preventDefault()
  if (!canManageTasks.value) return
  const taskId = event.dataTransfer?.getData('text/task-id') || draggingId.value
  draggingId.value = ''; dropStatusId.value = ''
  if (!taskId) return
  const task = boardStore.board?.tasks.find((item) => item.id === taskId)
  if (!task || task.status_id === statusId) return
  try {
    await boardStore.transition(taskId, statusId)
    toasts.show('Task moved', { tone: 'success', message: task.title })
  } catch (error) {
    toasts.show('Transition blocked', { tone: 'error', message: error instanceof Error ? error.message : 'The workflow rule did not allow this move.', timeout: 6500 })
  }
}

async function createStatus(input: { name: string }) {
  settingsBusy.value = true
  const nextPosition = Math.max(-1, ...boardStore.orderedStatuses.map((status) => status.position)) + 1
  try { await boardStore.createStatus({ ...input, position: nextPosition }); toasts.show('Column added', { tone: 'success' }) }
  catch (error) { toasts.show('Could not add column', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { settingsBusy.value = false }
}
async function updateStatus(id: string, input: { name?: string; position?: number }) {
  settingsBusy.value = true
  try { await boardStore.updateStatus(id, input) }
  catch (error) { toasts.show('Could not update column', { tone: 'error', message: error instanceof Error ? error.message : undefined }); await boardStore.load(boardId.value, true).catch(() => undefined) }
  finally { settingsBusy.value = false }
}
async function reorderStatuses(first: BoardStatus, second: BoardStatus) {
  settingsBusy.value = true
  const firstPosition = first.position
  const secondPosition = second.position
  first.position = secondPosition
  second.position = firstPosition
  try {
    await boardStore.updateStatus(first.id, { position: secondPosition })
    await boardStore.updateStatus(second.id, { position: firstPosition })
    await boardStore.load(boardId.value, true)
  } catch (error) {
    toasts.show('Could not reorder columns', { tone: 'error', message: error instanceof Error ? error.message : undefined })
    await boardStore.load(boardId.value, true).catch(() => undefined)
  } finally { settingsBusy.value = false }
}
async function deleteStatus(status: BoardStatus) {
  const count = tasksByStatus.value[status.id]?.length || 0
  if (!window.confirm(`Delete “${status.name}”?${count ? ` It contains ${count} task(s).` : ''}`)) return
  settingsBusy.value = true
  try { await boardStore.deleteStatus(status.id); toasts.show('Column deleted', { tone: 'success' }) }
  catch (error) { toasts.show('Could not delete column', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { settingsBusy.value = false }
}
async function createRule(input: { from_status_id: string; to_status_id: string; conditions: WorkflowConditions }) {
  settingsBusy.value = true
  try { await boardStore.createRule(input); toasts.show('Workflow rule added', { tone: 'success' }) }
  catch (error) { toasts.show('Could not add rule', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { settingsBusy.value = false }
}
async function deleteRule(rule: WorkflowRule) {
  if (!window.confirm('Delete this workflow rule?')) return
  settingsBusy.value = true
  try { await boardStore.deleteRule(rule.id); toasts.show('Workflow rule deleted', { tone: 'success' }) }
  catch (error) { toasts.show('Could not delete rule', { tone: 'error', message: error instanceof Error ? error.message : undefined }) }
  finally { settingsBusy.value = false }
}
</script>

<template>
  <main class="board-page">
    <header class="board-header">
      <div class="board-header__title">
        <button class="icon-button icon-button--bordered" aria-label="Back to projects" @click="router.push('/projects')"><AppIcon name="back" :size="18" /></button>
        <div><p class="breadcrumb"><span>{{ project?.name || 'Project' }}</span><i>/</i> Board</p><h1>{{ boardStore.board?.name || 'Board' }}</h1></div>
      </div>
      <div class="board-header__actions">
        <select v-if="boardOptions.length > 1" :value="boardId" aria-label="Switch board" @change="router.push(`/projects/${projectId}/boards/${($event.target as HTMLSelectElement).value}`)"><option v-for="option in boardOptions" :key="option.id" :value="option.id">{{ option.name }}</option></select>
        <button v-if="canManageBoard" class="button button--secondary" data-testid="board-settings-button" @click="showSettings = true"><AppIcon name="settings" :size="17" /> Settings</button>
        <button v-if="canManageTasks" class="button button--primary" data-testid="new-task" :disabled="!boardStore.orderedStatuses.length" @click="openNewTask()"><AppIcon name="plus" :size="18" /> New task</button>
      </div>
    </header>

    <div v-if="boardStore.error" class="alert alert--error board-alert"><span>{{ boardStore.error }}</span><button class="text-button" @click="boardStore.load(boardId)">Try again</button></div>
    <section v-if="boardStore.loading" class="kanban kanban--loading"><div v-for="i in 3" :key="i" class="kanban-column skeleton"></div></section>
    <section v-else-if="!boardStore.orderedStatuses.length" class="empty-state empty-state--board">
      <span class="empty-state__art"><AppIcon name="settings" :size="34" /></span><h2>This board needs columns</h2><p>Add statuses such as To do, In progress, and Done to start planning work.</p><button v-if="canManageBoard" class="button button--primary" @click="showSettings = true">Set up columns</button>
    </section>
    <section v-else class="kanban" data-testid="kanban-board">
      <article
        v-for="status in boardStore.orderedStatuses"
        :key="status.id"
        class="kanban-column"
        :class="{ 'kanban-column--drop': dropStatusId === status.id }"
        :data-status-id="status.id"
        data-testid="kanban-column"
        @dragover.prevent="canManageTasks && (dropStatusId = status.id)"
        @dragleave.self="dropStatusId = ''"
        @drop="dropTask($event, status.id)"
      >
        <header><div><span class="status-dot" :style="{ background: status.color || 'var(--accent)' }"></span><h2>{{ status.name }}</h2><span class="task-count">{{ tasksByStatus[status.id]?.length || 0 }}</span></div><button v-if="canManageTasks" class="icon-button" :aria-label="`Add task to ${status.name}`" @click="openNewTask(status.id)"><AppIcon name="plus" :size="18" /></button></header>
        <div class="task-list">
          <TaskCard v-for="task in tasksByStatus[status.id] || []" :key="task.id" :task="task" :moving="boardStore.movingTaskId === task.id" :readonly="!canManageTasks" @open="openTask" @dragstart="startDrag" />
          <button v-if="canManageTasks && !(tasksByStatus[status.id]?.length)" class="column-empty" @click="openNewTask(status.id)"><AppIcon name="plus" :size="16" /> Add the first task</button>
          <p v-else-if="!canManageTasks && !(tasksByStatus[status.id]?.length)" class="empty-inline">No tasks</p>
        </div>
      </article>
    </section>

    <TaskModal v-if="modalOpen" :key="modalKey" :task="selectedTask" :statuses="boardStore.orderedStatuses" :initial-status-id="initialStatusId" :members="members" :busy="saving" :readonly="!canManageTasks" @close="modalOpen = false" @submit="saveTask" />
    <BoardSettings v-if="showSettings" :statuses="boardStore.orderedStatuses" :rules="boardStore.board?.rules || []" :busy="settingsBusy" @close="showSettings = false" @create-status="createStatus" @update-status="updateStatus" @reorder-statuses="reorderStatuses" @delete-status="deleteStatus" @create-rule="createRule" @delete-rule="deleteRule" />
  </main>
</template>
