import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { taskflowApi } from '../api/taskflow'
import type { Board, Task } from '../domain/types'
import { useBoardStore } from './board'

vi.mock('../api/taskflow', () => ({
  taskflowApi: {
    addComment: vi.fn(),
    updateTask: vi.fn(),
    transition: vi.fn(),
  },
}))

const addCommentMock = vi.mocked(taskflowApi.addComment)
const updateTaskMock = vi.mocked(taskflowApi.updateTask)
const transitionMock = vi.mocked(taskflowApi.transition)

function seedBoard(): Board {
  return {
    id: 'board-1',
    project_id: 'project-1',
    name: 'Board',
    statuses: [],
    rules: [],
    tasks: [{ id: 'task-1', board_id: 'board-1', status_id: 'todo', title: 'Original title', comments: [] }],
  }
}

describe('board store: saveTaskEdits', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    addCommentMock.mockReset()
    updateTaskMock.mockReset()
    transitionMock.mockReset()
  })

  it('saves the comment, then the fields, then transitions — in that order — on full success', async () => {
    const store = useBoardStore()
    store.board = seedBoard()
    const order: string[] = []
    addCommentMock.mockImplementation(async () => { order.push('comment'); return { id: 'c1', body: 'Looks good' } })
    updateTaskMock.mockImplementation(async (_id, input) => { order.push('fields'); return { id: 'task-1', status_id: 'todo', ...input } as Task })
    transitionMock.mockImplementation(async () => { order.push('transition'); return { id: 'task-1', status_id: 'done' } as Task })

    const result = await store.saveTaskEdits('task-1', {
      title: 'New title',
      comment: 'Looks good',
      statusChanged: true,
      targetStatusId: 'done',
    })

    expect(order).toEqual(['comment', 'fields', 'transition'])
    expect(result).toEqual({ status: 'saved' })
    expect(store.board?.tasks[0].title).toBe('New title')
    expect(store.board?.tasks[0].status_id).toBe('done')
  })

  it('keeps saved fields but reverts the status when the transition is blocked, without throwing', async () => {
    const store = useBoardStore()
    store.board = seedBoard()
    updateTaskMock.mockResolvedValue({ id: 'task-1', status_id: 'todo', title: 'New title' } as Task)
    transitionMock.mockRejectedValue(new Error('A comment is required before this transition.'))

    const result = await store.saveTaskEdits('task-1', {
      title: 'New title',
      statusChanged: true,
      targetStatusId: 'done',
    })

    expect(result).toEqual({ status: 'blocked', reason: 'A comment is required before this transition.' })
    // The field edit landed on the task actually held in board.tasks...
    expect(store.board?.tasks[0].title).toBe('New title')
    // ...and the status was reverted to its original value on that same task, not left
    // on the rejected target. This is the identity-sensitive path the modal's forced
    // remount depends on to show the correct (reverted) status.
    expect(store.board?.tasks[0].status_id).toBe('todo')
    expect(store.board?.tasks[0].id).toBe('task-1')
    expect(addCommentMock).not.toHaveBeenCalled()
  })

  it('does not attempt a transition when the status did not change', async () => {
    const store = useBoardStore()
    store.board = seedBoard()
    updateTaskMock.mockResolvedValue({ id: 'task-1', status_id: 'todo', title: 'New title' } as Task)

    const result = await store.saveTaskEdits('task-1', {
      title: 'New title',
      statusChanged: false,
      targetStatusId: 'todo',
    })

    expect(result).toEqual({ status: 'saved' })
    expect(transitionMock).not.toHaveBeenCalled()
  })

  it('propagates a field-save failure instead of swallowing it, and never attempts the transition', async () => {
    const store = useBoardStore()
    store.board = seedBoard()
    updateTaskMock.mockRejectedValue(new Error('Title is required'))

    await expect(store.saveTaskEdits('task-1', {
      title: '',
      statusChanged: true,
      targetStatusId: 'done',
    })).rejects.toThrow('Title is required')

    expect(transitionMock).not.toHaveBeenCalled()
    // Field save failed outright, so the task in the store is untouched.
    expect(store.board?.tasks[0].title).toBe('Original title')
  })
})
