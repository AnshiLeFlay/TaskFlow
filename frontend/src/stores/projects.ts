import { defineStore } from 'pinia'
import { taskflowApi } from '../api/taskflow'
import type { Board, Project, ProjectMember, ProjectRole } from '../domain/types'

export const useProjectsStore = defineStore('projects', {
  state: () => ({
    projects: [] as Project[],
    boardsByProject: {} as Record<string, Board[]>,
    membersByProject: {} as Record<string, ProjectMember[]>,
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
      // Only append to an already-populated cache (i.e. loadMembers has genuinely
      // fetched this project before). Never *create* the cache entry here: doing so
      // would leave it containing only this one member, and loadMembers' "already
      // cached, skip the fetch" check would then serve that incomplete list forever.
      // Leaving the cache empty/absent lets the next loadMembers() do the real fetch.
      if (this.membersByProject[projectId]) this.membersByProject[projectId].push(member)
      return member
    },
    async loadMembers(projectId: string, force = false) {
      if (!force && this.membersByProject[projectId]) return this.membersByProject[projectId]
      const members = await taskflowApi.listMembers(projectId)
      this.membersByProject[projectId] = members
      return members
    },
  },
})
