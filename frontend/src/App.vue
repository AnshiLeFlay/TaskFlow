<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { validToken } from './api/keycloak'
import { AuthenticatedRealtimeSession, ProjectSocket } from './api/realtime'
import AppIcon from './components/AppIcon.vue'
import ToastStack from './components/ToastStack.vue'
import { handleRealtimeEvent } from './realtime/events'
import { useAuthStore } from './stores/auth'
import { useBoardStore } from './stores/board'
import { useToastStore } from './stores/toasts'

const auth = useAuthStore()
const boardStore = useBoardStore()
const toasts = useToastStore()
const route = useRoute()
const showShell = computed(() => route.name !== 'login' && auth.authenticated)

const realtime = new AuthenticatedRealtimeSession(() => new ProjectSocket(undefined, validToken, (event) => {
  handleRealtimeEvent(event, {
    activeBoardId: route.name === 'board' ? String(route.params.boardId || '') : '',
    board: boardStore.board,
    showToast: (title, options) => { toasts.show(title, options) },
    currentUserId: auth.user?.id,
  })
}))

watch(() => auth.authenticated, (authenticated) => realtime.sync(authenticated), { immediate: true })
onBeforeUnmount(() => realtime.disconnect())
</script>

<template>
  <div v-if="!auth.ready" class="app-loader" data-testid="app-loading">
    <span class="logo-mark"><i></i><i></i><i></i></span>
    <div class="spinner"></div>
  </div>
  <div v-else class="app-frame">
    <header v-if="showShell" class="topbar">
      <RouterLink class="brand" to="/projects" aria-label="TaskFlow projects">
        <span class="logo-mark logo-mark--small"><i></i><i></i><i></i></span>
        <span>Task<span>Flow</span></span>
      </RouterLink>
      <nav class="topbar__nav" aria-label="Main navigation">
        <RouterLink to="/projects"><AppIcon name="grid" :size="17" /> Projects</RouterLink>
      </nav>
      <div class="topbar__user">
        <div class="avatar" :title="auth.displayName">{{ auth.initials }}</div>
        <span class="topbar__name">{{ auth.displayName }}</span>
        <button class="icon-button" aria-label="Sign out" title="Sign out" @click="auth.logout()"><AppIcon name="logout" :size="18" /></button>
      </div>
    </header>
    <RouterView />
    <ToastStack />
  </div>
</template>
