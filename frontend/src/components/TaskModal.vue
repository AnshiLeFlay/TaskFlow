<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { BoardStatus, ProjectMember, Task } from '../domain/types'
import AppIcon from './AppIcon.vue'

const props = defineProps<{ task: Task | null; statuses: BoardStatus[]; initialStatusId?: string; members?: ProjectMember[]; busy?: boolean; readonly?: boolean }>()
const emit = defineEmits<{
  close: []
  submit: [value: { title: string; description?: string; status_id: string; assignee_id?: string | null; deadline?: string | null; comment?: string }]
}>()

const form = reactive({ title: '', description: '', status_id: '', assignee_id: '', deadline: '', comment: '' })
const editing = computed(() => Boolean(props.task))
const assigneeOptions = computed<Array<{ user_id: string; role: string }>>(() => {
  const list = props.members || []
  if (form.assignee_id && !list.some((member) => member.user_id === form.assignee_id)) {
    return [...list, { user_id: form.assignee_id, role: 'unknown' }]
  }
  return list
})

watch(() => [props.task, props.initialStatusId] as const, () => {
  form.title = props.task?.title || ''
  form.description = props.task?.description || ''
  form.status_id = props.task?.status_id || props.initialStatusId || props.statuses[0]?.id || ''
  form.assignee_id = props.task?.assignee_id || ''
  const rawDeadline = props.task?.deadline || props.task?.due_date
  form.deadline = rawDeadline ? rawDeadline.slice(0, 10) : ''
  form.comment = ''
}, { immediate: true })

function submit() {
  if (props.readonly || !form.title.trim() || !form.status_id) return
  emit('submit', {
    title: form.title.trim(),
    description: form.description.trim() || undefined,
    status_id: form.status_id,
    assignee_id: form.assignee_id.trim() || null,
    deadline: form.deadline ? new Date(`${form.deadline}T23:59:59`).toISOString() : null,
    comment: form.comment.trim() || undefined,
  })
}
</script>

<template>
  <div class="modal-backdrop" @mousedown.self="emit('close')">
    <form class="modal task-modal" data-testid="task-modal" @submit.prevent="submit">
      <div class="modal__head">
        <div><p class="eyebrow">{{ editing ? `Task #${task?.id.slice(0, 6)}` : 'New task' }}</p><h2>{{ editing ? 'Task details' : 'Create a task' }}</h2></div>
        <button type="button" class="icon-button" aria-label="Close" @click="emit('close')"><AppIcon name="close" /></button>
      </div>
      <div class="task-form-grid">
        <div class="task-form-main">
          <label class="field"><span>Title</span><input v-model="form.title" :readonly="readonly" required maxlength="200" autofocus placeholder="What needs to be done?" data-testid="task-title" /></label>
          <label class="field"><span>Description <small>optional</small></span><textarea v-model="form.description" :readonly="readonly" rows="5" maxlength="4000" placeholder="Add context, links, and acceptance criteria…"></textarea></label>
          <section v-if="editing" class="comments-section">
            <div class="section-label"><span>Activity</span><small>{{ task?.comments?.length || 0 }} comments</small></div>
            <div v-if="task?.comments?.length" class="comment-list">
              <article v-for="item in task.comments" :key="item.id" class="comment"><span class="mini-avatar">{{ (item.author_name || 'U')[0].toUpperCase() }}</span><div><strong>{{ item.author_name || 'Team member' }}</strong><p>{{ item.body || item.content }}</p></div></article>
            </div>
            <label v-if="!readonly" class="field"><span>Add a comment</span><textarea v-model="form.comment" rows="2" placeholder="Share an update…" data-testid="task-comment"></textarea></label>
          </section>
        </div>
        <aside class="task-form-side">
          <label class="field"><span>Status</span><select v-model="form.status_id" :disabled="readonly" required data-testid="task-status"><option v-for="status in statuses" :key="status.id" :value="status.id">{{ status.name }}</option></select></label>
          <label class="field"><span>Assignee</span><select v-model="form.assignee_id" :disabled="readonly" data-testid="task-assignee"><option value="">Unassigned</option><option v-for="member in assigneeOptions" :key="member.user_id" :value="member.user_id">{{ member.role }} — {{ member.user_id }}</option></select></label>
          <label class="field"><span>Deadline</span><input v-model="form.deadline" :disabled="readonly" type="date" data-testid="task-deadline" /></label>
          <div v-if="editing && task?.author_id" class="meta-note"><span>Created by</span><code>{{ task.author_id }}</code></div>
        </aside>
      </div>
      <div class="modal__actions"><button type="button" class="button button--ghost" @click="emit('close')">{{ readonly ? 'Close' : 'Cancel' }}</button><button v-if="!readonly" class="button button--primary" :disabled="busy || !form.title.trim() || !form.status_id">{{ busy ? 'Saving…' : editing ? 'Save changes' : 'Create task' }}</button></div>
    </form>
  </div>
</template>
