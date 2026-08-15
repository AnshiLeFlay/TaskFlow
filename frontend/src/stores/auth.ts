import { defineStore } from 'pinia'
import type { User } from '../domain/types'
import { initKeycloak, keycloak } from '../api/keycloak'
import { taskflowApi } from '../api/taskflow'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    ready: false,
    authenticated: false,
    user: null as User | null,
    error: '',
  }),
  getters: {
    displayName: (state) => state.user?.name || state.user?.username || state.user?.email || keycloak.tokenParsed?.preferred_username || 'User',
    initials(): string {
      return String(this.displayName).split(/\s+/).slice(0, 2).map((part: string) => part[0]?.toUpperCase()).join('') || 'U'
    },
  },
  actions: {
    async initialize() {
      if (this.ready) return this.authenticated
      try {
        this.authenticated = await initKeycloak()
        if (this.authenticated) {
          keycloak.onTokenExpired = () => { void keycloak.updateToken(30) }
          try {
            this.user = await taskflowApi.me()
          } catch {
            this.user = {
              id: String(keycloak.subject || ''),
              username: String(keycloak.tokenParsed?.preferred_username || ''),
              email: String(keycloak.tokenParsed?.email || ''),
              name: String(keycloak.tokenParsed?.name || ''),
            }
          }
        }
      } catch (error) {
        this.error = error instanceof Error ? error.message : 'Authentication service is unavailable'
      } finally {
        this.ready = true
      }
      return this.authenticated
    },
    login(redirectPath = '/projects') {
      return keycloak.login({ redirectUri: `${window.location.origin}${redirectPath}` })
    },
    register() {
      return keycloak.register({ redirectUri: `${window.location.origin}/projects` })
    },
    logout() {
      this.authenticated = false
      this.user = null
      return keycloak.logout({ redirectUri: `${window.location.origin}/login` })
    },
  },
})
