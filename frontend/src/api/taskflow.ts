import type { Board, BoardStatus, Project, ProjectMember, Task, User, WorkflowConditions, WorkflowRule } from '../domain/types'
import { request, unwrapItem, unwrapList } from './http'

export const taskflowApi = {
  async me(): Promise<User> {
    return unwrapItem(await request<User | Record<string, unknown>>('/me'), 'user')
  },
  async users(): Promise<User[]> {
    return unwrapList(await request<User[] | Record<string, unknown>>('/users'), 'users')
  },
  async projects(): Promise<Project[]> {
    return unwrapList(await request<Project[] | Record<string, unknown>>('/projects'), 'projects')
  },
  async createProject(input: { name: string; description?: string }): Promise<Project> {
    return unwrapItem(await request<Project | Record<string, unknown>>('/projects', { method: 'POST', body: JSON.stringify(input) }), 'project')
  },
  async addMember(projectId: string, input: Pick<ProjectMember, 'user_id' | 'role'>): Promise<ProjectMember> {
    return unwrapItem(await request<ProjectMember | Record<string, unknown>>(`/projects/${projectId}/members`, { method: 'POST', body: JSON.stringify(input) }), 'member')
  },
  async listMembers(projectId: string): Promise<ProjectMember[]> {
    return unwrapList(await request<ProjectMember[] | Record<string, unknown>>(`/projects/${projectId}/members`), 'members')
  },
  async boards(projectId: string): Promise<Board[]> {
    return unwrapList(await request<Board[] | Record<string, unknown>>(`/projects/${projectId}/boards`), 'boards')
  },
  async createBoard(projectId: string, input: { name: string; description?: string }): Promise<Board> {
    return unwrapItem(await request<Board | Record<string, unknown>>(`/projects/${projectId}/boards`, { method: 'POST', body: JSON.stringify(input) }), 'board')
  },
  async board(boardId: string): Promise<Board> {
    const board = unwrapItem<Board>(await request<Board | Record<string, unknown>>(`/boards/${boardId}`), 'board')
    return { ...board, statuses: board.statuses || [], tasks: board.tasks || [], rules: board.rules || [] }
  },
  async createStatus(boardId: string, input: { name: string; position: number }): Promise<BoardStatus> {
    return unwrapItem(await request<BoardStatus | Record<string, unknown>>(`/boards/${boardId}/statuses`, { method: 'POST', body: JSON.stringify(input) }), 'status')
  },
  async updateStatus(statusId: string, input: Partial<Pick<BoardStatus, 'name' | 'position' | 'color'>>): Promise<BoardStatus> {
    return unwrapItem(await request<BoardStatus | Record<string, unknown>>(`/statuses/${statusId}`, { method: 'PATCH', body: JSON.stringify(input) }), 'status')
  },
  deleteStatus(statusId: string): Promise<void> {
    return request(`/statuses/${statusId}`, { method: 'DELETE' })
  },
  async createRule(boardId: string, input: { from_status_id: string; to_status_id: string; conditions: WorkflowConditions }): Promise<WorkflowRule> {
    return unwrapItem(await request<WorkflowRule | Record<string, unknown>>(`/boards/${boardId}/rules`, { method: 'POST', body: JSON.stringify(input) }), 'rule')
  },
  async updateRule(ruleId: string, input: Partial<WorkflowRule>): Promise<WorkflowRule> {
    return unwrapItem(await request<WorkflowRule | Record<string, unknown>>(`/rules/${ruleId}`, { method: 'PATCH', body: JSON.stringify(input) }), 'rule')
  },
  deleteRule(ruleId: string): Promise<void> {
    return request(`/rules/${ruleId}`, { method: 'DELETE' })
  },
  async createTask(boardId: string, input: Omit<Partial<Task>, 'id' | 'board_id'> & { title: string; status_id: string }): Promise<Task> {
    return unwrapItem(await request<Task | Record<string, unknown>>(`/boards/${boardId}/tasks`, { method: 'POST', body: JSON.stringify(input) }), 'task')
  },
  async updateTask(taskId: string, input: Partial<Task>): Promise<Task> {
    return unwrapItem(await request<Task | Record<string, unknown>>(`/tasks/${taskId}`, { method: 'PATCH', body: JSON.stringify(input) }), 'task')
  },
  async addComment(taskId: string, content: string): Promise<unknown> {
    return request(`/tasks/${taskId}/comments`, { method: 'POST', body: JSON.stringify({ body: content }) })
  },
  async transition(taskId: string, targetStatusId: string): Promise<Task> {
    return unwrapItem(await request<Task | Record<string, unknown>>(`/tasks/${taskId}/transition`, { method: 'POST', body: JSON.stringify({ target_status_id: targetStatusId }) }), 'task')
  },
}
