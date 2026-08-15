<script setup lang="ts">
import { useRoute } from 'vue-router'
import AppIcon from '../components/AppIcon.vue'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()
const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/projects'
</script>

<template>
  <main class="login-page">
    <section class="login-copy">
      <a class="brand brand--light" href="/">
        <span class="logo-mark"><i></i><i></i><i></i></span>
        <span>Task<span>Flow</span></span>
      </a>
      <div class="login-copy__center">
        <span class="eyebrow">Projects, without the noise</span>
        <h1>Make work<br><em>flow.</em></h1>
        <p>A calm, focused workspace for teams that want clear ownership and flexible workflows.</p>
      </div>
      <div class="board-preview" aria-hidden="true">
        <div><span></span><b></b><b></b></div><div><span></span><b></b></div><div><span></span><b></b><b></b></div>
      </div>
    </section>
    <section class="login-panel">
      <div class="login-card">
        <span class="login-card__mobile-logo logo-mark"><i></i><i></i><i></i></span>
        <p class="eyebrow">Welcome to TaskFlow</p>
        <h2>Ready when you are.</h2>
        <p class="muted">Sign in to pick up where your team left off.</p>
        <div v-if="auth.error" class="alert alert--error">{{ auth.error }}</div>
        <button class="button button--primary button--wide" data-testid="login-button" @click="auth.login(redirect)">
          Sign in with Keycloak <AppIcon name="arrow" :size="18" />
        </button>
        <div class="divider"><span>New to TaskFlow?</span></div>
        <button class="button button--secondary button--wide" data-testid="register-button" @click="auth.register()">Create an account</button>
        <p class="login-card__legal">Secure identity is provided by Keycloak.</p>
      </div>
    </section>
  </main>
</template>
