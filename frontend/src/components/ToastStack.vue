<script setup lang="ts">
import { useToastStore } from '../stores/toasts'
import AppIcon from './AppIcon.vue'
const toasts = useToastStore()
</script>

<template>
  <div class="toast-stack" aria-live="polite" aria-relevant="additions">
    <TransitionGroup name="toast">
      <article v-for="toast in toasts.items" :key="toast.id" class="toast" :class="`toast--${toast.tone}`" role="status" data-testid="toast">
        <span class="toast__mark"><AppIcon :name="toast.tone === 'success' ? 'check' : toast.tone === 'error' ? 'close' : 'message'" :size="16" /></span>
        <div><strong>{{ toast.title }}</strong><p v-if="toast.message">{{ toast.message }}</p></div>
        <button class="icon-button toast__close" aria-label="Dismiss notification" @click="toasts.dismiss(toast.id)"><AppIcon name="close" :size="16" /></button>
      </article>
    </TransitionGroup>
  </div>
</template>
