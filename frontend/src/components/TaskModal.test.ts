import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { BoardStatus, ProjectMember, Task } from '../domain/types'
import TaskModal from './TaskModal.vue'

const statuses: BoardStatus[] = [
  { id: 'todo', name: 'To Do', position: 0 },
  { id: 'in-progress', name: 'In Progress', position: 1 },
]

function task(overrides: Partial<Task> = {}): Task {
  return { id: 'task-1', board_id: 'board-1', status_id: 'todo', title: 'Ship it', comments: [], ...overrides }
}

describe('TaskModal', () => {
  it('initializes the status select from the task prop', () => {
    const wrapper = mount(TaskModal, { props: { task: task({ status_id: 'in-progress' }), statuses } })
    expect((wrapper.get('[data-testid="task-status"]').element as HTMLSelectElement).value).toBe('in-progress')
  })

  it('a freshly mounted instance reflects a reverted status, even after the user picked a different one', async () => {
    // This is what BoardView's saveTask relies on: after a blocked transition, the store
    // reverts task.status_id to its real value and BoardView forces a remount (:key bump)
    // so the modal picks that reverted value back up rather than showing the rejected one.
    const original = mount(TaskModal, { props: { task: task({ status_id: 'todo' }), statuses } })
    await original.get('[data-testid="task-status"]').setValue('in-progress')
    expect((original.get('[data-testid="task-status"]').element as HTMLSelectElement).value).toBe('in-progress')
    original.unmount()

    const remounted = mount(TaskModal, { props: { task: task({ status_id: 'todo' }), statuses } })
    expect((remounted.get('[data-testid="task-status"]').element as HTMLSelectElement).value).toBe('todo')
  })

  it('lists assignable project members by identity, plus a default Unassigned option', () => {
    const members: ProjectMember[] = [{ user_id: 'user-1', username: 'alice', role: 'admin' }, { user_id: 'user-2', name: 'Bob Member', role: 'member' }]
    const wrapper = mount(TaskModal, { props: { task: task(), statuses, members } })
    const options = wrapper.findAll('[data-testid="task-assignee"] option')
    expect(options.map((option) => option.text())).toEqual(['Unassigned', 'alice (admin)', 'Bob Member (member)'])
  })

  it('does not offer read-only viewers as assignees', () => {
    const members: ProjectMember[] = [{ user_id: 'viewer-1', username: 'vera', role: 'viewer' }]
    const wrapper = mount(TaskModal, { props: { task: task(), statuses, members } })
    expect(wrapper.findAll('[data-testid="task-assignee"] option').map((option) => option.text())).toEqual(['Unassigned'])
  })

  it('falls back to just Unassigned when no members have loaded yet', () => {
    const wrapper = mount(TaskModal, { props: { task: task(), statuses } })
    const options = wrapper.findAll('[data-testid="task-assignee"] option')
    expect(options.map((option) => option.text())).toEqual(['Unassigned'])
  })

  it('keeps an already-assigned user selectable even if they are missing from the loaded members list', () => {
    const wrapper = mount(TaskModal, { props: { task: task({ assignee_id: 'user-9' }), statuses, members: [] } })
    expect((wrapper.get('[data-testid="task-assignee"]').element as HTMLSelectElement).value).toBe('user-9')
  })
})
