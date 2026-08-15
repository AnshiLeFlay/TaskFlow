import { defineStore } from 'pinia'
import { taskflowApi } from '../api/taskflow'
import type { Board, Project, ProjectMember, ProjectRole } from '../domain/types'

export const useProjectsStore = defineStore('projects', {
  state: () => ({
    projects: [] as Project[],
    boardsByProject: {} as Record<string, Board[]>,
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
      return taskflowApi.addMember(projectId, { user_id: userId, role })
    },
  },
})
