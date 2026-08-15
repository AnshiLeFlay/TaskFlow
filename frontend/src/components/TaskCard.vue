<script setup lang="ts">
import { computed } from 'vue'
import type { Task } from '../domain/types'
import AppIcon from './AppIcon.vue'

const props = defineProps<{ task: Task; moving?: boolean; readonly?: boolean }>()
defineEmits<{ open: [task: Task]; dragstart: [event: DragEvent, task: Task] }>()

const deadline = computed(() => {
  const raw = props.task.deadline || props.task.due_date
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.valueOf())) return raw
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(date)
})
const overdue = computed(() => Boolean((props.task.deadline || props.task.due_date) && new Date(props.task.deadline || props.task.due_date || '').valueOf() < Date.now()))
const assignee = computed(() => props.task.assignee?.name || props.task.assignee?.username || props.task.assignee?.email || props.task.assignee_id || '')
const initials = computed(() => assignee.value.split(/[@\s._-]/).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase()).join(''))
</script>

<template>
  <article
    class="task-card"
    :class="{ 'task-card--moving': moving, 'task-card--readonly': readonly }"
    :draggable="!readonly"
    :data-task-id="task.id"
    data-testid="task-card"
    tabindex="0"
    @click="$emit('open', task)"
    @keydown.enter="$emit('open', task)"
    @dragstart="!readonly && $emit('dragstart', $event, task)"
  >
    <div class="task-card__top"><span v-if="task.id" class="task-key">#{{ task.id.slice(0, 6) }}</span><AppIcon name="dots" :size="18" /></div>
    <h3>{{ task.title }}</h3>
    <p v-if="task.description">{{ task.description }}</p>
    <footer>
      <span v-if="deadline" class="task-date" :class="{ 'task-date--overdue': overdue }"><AppIcon name="calendar" :size="14" />{{ deadline }}</span>
      <span v-if="task.comments?.length" class="task-comments"><AppIcon name="message" :size="14" />{{ task.comments.length }}</span>
      <span v-if="assignee" class="mini-avatar" :title="assignee">{{ initials }}</span>
      <span v-else class="unassigned" title="Unassigned"><AppIcon name="user" :size="15" /></span>
    </footer>
  </article>
</template>
