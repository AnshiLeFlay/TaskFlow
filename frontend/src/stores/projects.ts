import { defineStore } from 'pinia'
import { taskflowApi } from '../api/taskflow'
import type { Board, Project, ProjectMember, ProjectRole, User } from '../domain/types'

export const useProjectsStore = defineStore('projects', {
  state: () => ({
    projects: [] as Project[],
    boardsByProject: {} as Record<string, Board[]>,
    membersByProject: {} as Record<string, ProjectMember[]>,
    users: [] as User[],
    loading: false,
    error: '',
  }),
  actions: {
    async load() {
      this.loading = true
      this.error = ''
      try { this.projects = await taskflowApi.projects() }
      catch (error) { this.error = error instanceof Error ? error.message : 'Could not load projects'; throw error }
      finally { this.loading = false }
    },
    async create(input: { name: string; description?: string }) {
      const project = await taskflowApi.createProject(input)
      this.projects.unshift(project)
      this.boardsByProject[project.id] = []
      return project
    },
    async loadBoards(projectId: string, force = false) {
      if (!force && this.boardsByProject[projectId]) return this.boardsByProject[projectId]
      const boards = await taskflowApi.boards(projectId)
      this.boardsByProject[projectId] = boards
      return boards
    },
    async createBoard(projectId: string, input: { name: string; description?: string }) {
      const board = await taskflowApi.createBoard(projectId, input)
      this.boardsByProject[projectId] ||= []
      this.boardsByProject[projectId].push(board)
      return board
    },
    async addMember(projectId: string, userId: string, role: ProjectRole): Promise<ProjectMember> {
      const member = await taskflowApi.addMember(projectId, { user_id: userId, role })
      // Refresh so the persistent list immediately contains the new member together
      // with the identity fields supplied by the directory-enriched endpoint.
      await this.loadMembers(projectId, true)
      return member
    },
    async loadUsers(force = false) {
      if (!force && this.users.length) return this.users
      this.users = await taskflowApi.users()
      return this.users
    },
    async loadMembers(projectId: string, force = false) {
      if (!force && this.membersByProject[projectId]) return this.membersByProject[projectId]
      const members = await taskflowApi.listMembers(projectId)
      this.membersByProject[projectId] = members
      return members
    },
  },
})
