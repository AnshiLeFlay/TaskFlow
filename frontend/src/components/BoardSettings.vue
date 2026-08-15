<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { BoardStatus, ProjectRole, WorkflowCondition, WorkflowConditions, WorkflowRule } from '../domain/types'
import AppIcon from './AppIcon.vue'

const props = defineProps<{ statuses: BoardStatus[]; rules: WorkflowRule[]; busy?: boolean }>()
const emit = defineEmits<{
  close: []
  createStatus: [value: { name: string }]
  updateStatus: [id: string, value: { name?: string; position?: number }]
  reorderStatuses: [first: BoardStatus, second: BoardStatus]
  deleteStatus: [status: BoardStatus]
  createRule: [value: { from_status_id: string; to_status_id: string; conditions: WorkflowConditions }]
  deleteRule: [rule: WorkflowRule]
}>()

const tab = ref<'columns' | 'workflow'>('columns')
const newStatus = reactive({ name: '' })
const editingStatus = ref('')
const editName = ref('')
const rule = reactive({ from: '', to: '', condition: 'author_only' as WorkflowCondition, roles: ['admin'] as ProjectRole[] })
const ordered = computed(() => [...props.statuses].sort((a, b) => a.position - b.position))
const statusName = (id: string) => props.statuses.find((status) => status.id === id)?.name || 'Unknown'
const conditionLabel = (item: WorkflowRule) => {
  const value = item.conditions?.author_only ? 'author_only'
    : item.conditions?.assignee_only ? 'assignee_only'
      : item.conditions?.requires_comment ? 'requires_comment'
        : item.conditions?.project_owner_only ? 'project_owner_only'
          : item.conditions?.allowed_roles?.length ? 'allowed_roles'
            : item.condition_type || 'none'
  return ({ author_only: 'Task author only', assignee_only: 'Assignee only', requires_comment: 'Requires a comment', project_owner_only: 'Project owner only', allowed_roles: 'Selected roles', none: 'No condition' } as Record<string, string>)[value] || value
}

function addStatus() {
  if (!newStatus.name.trim()) return
  emit('createStatus', { name: newStatus.name.trim() })
  newStatus.name = ''
}
function startEdit(status: BoardStatus) { editingStatus.value = status.id; editName.value = status.name }
function saveEdit(status: BoardStatus) { if (editName.value.trim()) emit('updateStatus', status.id, { name: editName.value.trim() }); editingStatus.value = '' }
function move(status: BoardStatus, direction: -1 | 1) {
  const index = ordered.value.findIndex((item) => item.id === status.id)
  const swap = ordered.value[index + direction]
  if (!swap) return
  emit('reorderStatuses', status, swap)
}
function addRule() {
  if (!rule.from || !rule.to || rule.from === rule.to) return
  const condition: WorkflowConditions = rule.condition === 'allowed_roles'
    ? { allowed_roles: [...rule.roles] }
    : { [rule.condition]: true }
  emit('createRule', { from_status_id: rule.from, to_status_id: rule.to, conditions: condition })
}
</script>

<template>
  <div class="drawer-backdrop" @mousedown.self="emit('close')">
    <aside class="settings-drawer" data-testid="board-settings">
      <header><div><p class="eyebrow">Board setup</p><h2>Settings</h2></div><button class="icon-button" aria-label="Close settings" @click="emit('close')"><AppIcon name="close" /></button></header>
      <div class="tabs"><button :class="{ active: tab === 'columns' }" @click="tab = 'columns'">Columns</button><button :class="{ active: tab === 'workflow' }" @click="tab = 'workflow'">Workflow rules</button></div>

      <div v-if="tab === 'columns'" class="settings-content">
        <div class="settings-intro"><h3>Board columns</h3><p>Rename and reorder the statuses tasks move through.</p></div>
        <div class="status-list">
          <div v-for="(status, index) in ordered" :key="status.id" class="status-setting" :data-testid="`status-setting-${status.id}`">
            <span class="drag-handle">⋮⋮</span><span class="status-dot" :style="{ background: status.color || 'var(--accent)' }"></span>
            <input v-if="editingStatus === status.id" v-model="editName" autofocus @keyup.enter="saveEdit(status)" @keyup.escape="editingStatus = ''" />
            <strong v-else @dblclick="startEdit(status)">{{ status.name }}</strong>
            <div class="status-setting__actions">
              <button class="tiny-button" :disabled="index === 0 || busy" aria-label="Move column left" @click="move(status, -1)">←</button>
              <button class="tiny-button" :disabled="index === ordered.length - 1 || busy" aria-label="Move column right" @click="move(status, 1)">→</button>
              <button v-if="editingStatus === status.id" class="tiny-button" aria-label="Save column" @click="saveEdit(status)"><AppIcon name="check" :size="14" /></button>
              <button v-else class="tiny-button" aria-label="Rename column" @click="startEdit(status)">Aa</button>
              <button class="tiny-button tiny-button--danger" :disabled="busy" aria-label="Delete column" @click="emit('deleteStatus', status)"><AppIcon name="trash" :size="14" /></button>
            </div>
          </div>
        </div>
        <form class="inline-create inline-create--single" data-testid="status-form" @submit.prevent="addStatus">
          <input v-model="newStatus.name" required maxlength="80" placeholder="New column name" data-testid="status-name" />
          <button class="button button--secondary button--small" :disabled="busy"><AppIcon name="plus" :size="16" /> Add</button>
        </form>
      </div>

      <div v-else class="settings-content">
        <div class="settings-intro"><h3>Transition rules</h3><p>Define who can move a task and what must be true first.</p></div>
        <div v-if="rules.length" class="rule-list">
          <article v-for="item in rules" :key="item.id" class="rule-card" data-testid="workflow-rule">
            <div class="rule-card__route"><strong>{{ statusName(item.from_status_id) }}</strong><span>→</span><strong>{{ statusName(item.to_status_id) }}</strong></div>
            <p>{{ conditionLabel(item) }}</p>
            <button class="icon-button" aria-label="Delete rule" @click="emit('deleteRule', item)"><AppIcon name="trash" :size="16" /></button>
          </article>
        </div>
        <p v-else class="empty-inline empty-inline--box">No transitions are allowed yet. Add a rule below.</p>
        <form class="rule-form" data-testid="rule-form" @submit.prevent="addRule">
          <h4>New rule</h4>
          <div class="two-fields">
            <label class="field"><span>From</span><select v-model="rule.from" required data-testid="rule-from"><option value="" disabled>Select status</option><option v-for="status in ordered" :key="status.id" :value="status.id">{{ status.name }}</option></select></label>
            <label class="field"><span>To</span><select v-model="rule.to" required data-testid="rule-to"><option value="" disabled>Select status</option><option v-for="status in ordered" :key="status.id" :value="status.id">{{ status.name }}</option></select></label>
          </div>
          <label class="field"><span>Condition</span><select v-model="rule.condition" data-testid="rule-condition"><option value="author_only">Task author only</option><option value="assignee_only">Assignee only</option><option value="requires_comment">Requires at least one comment</option><option value="project_owner_only">Project owner only</option><option value="allowed_roles">Selected project roles</option></select></label>
          <fieldset v-if="rule.condition === 'allowed_roles'" class="role-checks"><legend>Allowed roles</legend><label v-for="roleName in (['admin', 'member', 'viewer'] as ProjectRole[])" :key="roleName"><input v-model="rule.roles" type="checkbox" :value="roleName" /> {{ roleName }}</label></fieldset>
          <button class="button button--primary button--wide" :disabled="busy || !rule.from || !rule.to || rule.from === rule.to">Add workflow rule</button>
        </form>
      </div>
    </aside>
  </div>
</template>
