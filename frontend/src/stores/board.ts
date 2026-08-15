import { defineStore } from 'pinia'
import { taskflowApi } from '../api/taskflow'
import type { Board, BoardStatus, Task, WorkflowConditions, WorkflowRule } from '../domain/types'

function normalizeBoard(board: Board): Board {
  const raw = board as Board & { columns?: BoardStatus[]; workflow_rules?: WorkflowRule[] }
  return {
    ...board,
    statuses: (board.statuses || raw.columns || []).map((status, index) => ({
      ...status,
      position: Number(status.position ?? (status as BoardStatus & { order?: number }).order ?? index),
    })),
    tasks: board.tasks || [],
    rules: board.rules || raw.workflow_rules || [],
  }
}

export const useBoardStore = defineStore('board', {
  state: () => ({
    board: null as Board | null,
    loading: false,
    error: '',
    movingTaskId: '',
  }),
  getters: {
    orderedStatuses: (state): BoardStatus[] => [...(state.board?.statuses || [])].sort((a, b) => a.position - b.position),
  },
  actions: {
    async load(boardId: string, quiet = false) {
      if (!quiet) this.loading = true
      this.error = ''
      try { this.board = normalizeBoard(await taskflowApi.board(boardId)) }
      catch (error) { this.error = error instanceof Error ? error.message : 'Could not load board'; throw error }
      finally { this.loading = false }
    },
    async createTask(input: Omit<Partial<Task>, 'id' | 'board_id'> & { title: string; status_id: string }) {
      if (!this.board) throw new Error('Board is not loaded')
      const task = await taskflowApi.createTask(this.board.id, input)
      this.board.tasks.push(task)
      return task
    },
    async updateTask(taskId: string, input: Partial<Task>) {
      const task = await taskflowApi.updateTask(taskId, input)
      if (this.board) {
        const index = this.board.tasks.findIndex((item) => item.id === taskId)
        if (index >= 0) this.board.tasks[index] = { ...this.board.tasks[index], ...task }
      }
      return task
    },
    async addComment(taskId: string, content: string) {
      const comment = await taskflowApi.addComment(taskId, content)
      if (this.board) {
        const task = this.board.tasks.find((item) => item.id === taskId)
        if (task && comment && typeof comment === 'object') {
          const body = comment as Record<string, unknown>
          const value = (body.comment || body.data || body) as NonNullable<Task['comments']>[number]
          if (!(task.comments || []).some((item) => item.id === value.id)) (task.comments ||= []).push(value)
        }
      }
      return comment
    },
    /**
     * Orchestrates the task-modal save flow for an existing task: comment, then field
     * edits, then (if the status changed) a transition attempt — in that order. Field
     * edits (and the comment) are persisted even if the transition is subsequently
     * rejected; only the transition's own failure is reported back as "blocked" rather
     * than thrown, so callers can keep their UI open and explain the partial success
     * without treating it as a hard error.
     */
    async saveTaskEdits(taskId: string, input: {
      title: string
      description?: string
      assignee_id?: string | null
      deadline?: string | null
      comment?: string
      statusChanged: boolean
      targetStatusId: string
    }): Promise<{ status: 'saved' } | { status: 'blocked'; reason?: string }> {
      if (input.comment) await this.addComment(taskId, input.comment)
      await this.updateTask(taskId, {
        title: input.title,
        description: input.description,
        assignee_id: input.assignee_id,
        deadline: input.deadline,
      })
      if (input.statusChanged) {
        try {
          await this.transition(taskId, input.targetStatusId)
        } catch (error) {
          return { status: 'blocked', reason: error instanceof Error ? error.message : undefined }
        }
      }
      return { status: 'saved' }
    },
    async transition(taskId: string, targetStatusId: string) {
      if (!this.board) throw new Error('Board is not loaded')
      const task = this.board.tasks.find((item) => item.id === taskId)
      if (!task || task.status_id === targetStatusId) return task
      const previousStatusId = task.status_id
      task.status_id = targetStatusId
      this.movingTaskId = taskId
      try {
        const updated = await taskflowApi.transition(taskId, targetStatusId)
        Object.assign(task, updated, { status_id: updated.status_id || targetStatusId })
        return task
      } catch (error) {
        task.status_id = previousStatusId
        throw error
      } finally { this.movingTaskId = '' }
    },
    async createStatus(input: { name: string; position: number }) {
      if (!this.board) throw new Error('Board is not loaded')
      const status = await taskflowApi.createStatus(this.board.id, input)
      this.board.statuses.push(status)
      return status
    },
    async updateStatus(statusId: string, input: Partial<Pick<BoardStatus, 'name' | 'position' | 'color'>>) {
      const status = await taskflowApi.updateStatus(statusId, input)
      if (this.board) {
        const current = this.board.statuses.find((item) => item.id === statusId)
        if (current) Object.assign(current, input, status)
      }
      return status
    },
    async deleteStatus(statusId: string) {
      await taskflowApi.deleteStatus(statusId)
      if (this.board) this.board.statuses = this.board.statuses.filter((item) => item.id !== statusId)
    },
    async createRule(input: { from_status_id: string; to_status_id: string; conditions: WorkflowConditions }) {
      if (!this.board) throw new Error('Board is not loaded')
      const rule = await taskflowApi.createRule(this.board.id, input)
      this.board.rules.push(rule)
      return rule
    },
    async deleteRule(ruleId: string) {
      await taskflowApi.deleteRule(ruleId)
      if (this.board) this.board.rules = this.board.rules.filter((item) => item.id !== ruleId)
    },
  },
})
