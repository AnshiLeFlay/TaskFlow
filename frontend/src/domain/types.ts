export type ProjectRole = 'admin' | 'member' | 'viewer'

export interface User {
  id: string
  username?: string
  email?: string
  name?: string
  roles?: ProjectRole[]
}

export interface ProjectMember {
  id?: string
  user_id: string
  username?: string
  email?: string
  name?: string
  role: ProjectRole
}

export interface Project {
  id: string
  name: string
  description?: string
  owner_id?: string
  role?: ProjectRole
  members?: ProjectMember[]
  created_at?: string
}

export interface BoardStatus {
  id: string
  board_id?: string
  name: string
  position: number
  color?: string
}

export type WorkflowCondition =
  | 'none'
  | 'author_only'
  | 'assignee_only'
  | 'requires_comment'
  | 'project_owner_only'
  | 'allowed_roles'

export interface WorkflowConditions {
  author_only?: boolean
  assignee_only?: boolean
  requires_comment?: boolean
  project_owner_only?: boolean
  allowed_roles?: ProjectRole[]
}

export interface WorkflowRule {
  id: string
  board_id?: string
  from_status_id: string
  to_status_id: string
  conditions: WorkflowConditions
  condition_type?: WorkflowCondition | string
  required_role?: ProjectRole
  description?: string
}

export interface Comment {
  id: string
  task_id?: string
  author_id?: string
  author_name?: string
  body: string
  content?: string
  created_at?: string
}

export interface Task {
  id: string
  board_id?: string
  status_id: string
  title: string
  description?: string
  author_id?: string
  assignee_id?: string | null
  assignee?: User | null
  deadline?: string | null
  due_date?: string | null
  comments?: Comment[]
  created_at?: string
  updated_at?: string
}

export interface Board {
  id: string
  project_id: string
  name: string
  description?: string
  statuses: BoardStatus[]
  tasks: Task[]
  rules: WorkflowRule[]
}

export interface RealtimeEvent {
  type: string
  event_type?: string
  project_id?: string
  board_id?: string
  task_id?: string
  task?: Task
  payload?: Record<string, unknown>
  message?: string
}
