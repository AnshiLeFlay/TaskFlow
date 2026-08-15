import { describe, expect, it, vi } from 'vitest'
import type { Board, RealtimeEvent } from '../domain/types'
import { handleRealtimeEvent } from './events'

function board(): Board {
  return {
    id: 'board-active',
    project_id: 'project-1',
    name: 'Active board',
    statuses: [],
    rules: [],
    tasks: [{ id: 'task-1', board_id: 'board-active', status_id: 'todo', title: 'Existing task', assignee_id: null, comments: [] }],
  }
}

describe('realtime event handling', () => {
  it('notifies globally but never inserts a task from a different board', () => {
    const activeBoard = board()
    const showToast = vi.fn()
    const foreignEvent: RealtimeEvent = {
      type: 'task.updated',
      project_id: 'project-1',
      board_id: 'board-other',
      task_id: 'task-other',
      payload: {
        task: { id: 'task-other', board_id: 'board-other', status_id: 'todo', title: 'Foreign task' },
      },
    }

    handleRealtimeEvent(foreignEvent, { activeBoardId: activeBoard.id, board: activeBoard, showToast })

    expect(activeBoard.tasks.map((task) => task.id)).toEqual(['task-1'])
    expect(showToast).toHaveBeenCalledWith('Task updated', expect.objectContaining({ tone: 'info' }))
  })

  it('upserts active-board task payloads without duplicates', () => {
    const activeBoard = board()
    const showToast = vi.fn()
    const event: RealtimeEvent = {
      type: 'task.transitioned',
      board_id: activeBoard.id,
      task_id: 'task-1',
      payload: {
        task: { id: 'task-1', board_id: activeBoard.id, status_id: 'done', title: 'Existing task' },
      },
    }

    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast })
    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast })

    expect(activeBoard.tasks).toHaveLength(1)
    expect(activeBoard.tasks[0].status_id).toBe('done')
    expect(showToast).toHaveBeenCalledTimes(2)
  })

  it('shows a distinct toast when task.updated changes the assignee', () => {
    const activeBoard = board()
    const showToast = vi.fn()
    const event: RealtimeEvent = {
      type: 'task.updated',
      board_id: activeBoard.id,
      task_id: 'task-1',
      payload: {
        task: { id: 'task-1', board_id: activeBoard.id, status_id: 'todo', title: 'Existing task', assignee_id: 'user-2' },
      },
    }

    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast })

    expect(activeBoard.tasks[0].assignee_id).toBe('user-2')
    expect(showToast).toHaveBeenCalledWith('Task assigned', expect.objectContaining({ tone: 'info', message: expect.stringContaining('Existing task') }))
  })

  it('says "You were assigned" when the current user becomes the assignee', () => {
    const activeBoard = board()
    const showToast = vi.fn()
    const event: RealtimeEvent = {
      type: 'task.updated',
      board_id: activeBoard.id,
      task_id: 'task-1',
      payload: {
        task: { id: 'task-1', board_id: activeBoard.id, status_id: 'todo', title: 'Existing task', assignee_id: 'user-2' },
      },
    }

    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast, currentUserId: 'user-2' })

    expect(showToast).toHaveBeenCalledWith('You were assigned', expect.objectContaining({ tone: 'info', message: expect.stringContaining('Existing task') }))
  })

  it('shows an unassignment toast — not an assignment toast — when the assignee is cleared', () => {
    const activeBoard = board()
    activeBoard.tasks[0].assignee_id = 'user-1'
    const showToast = vi.fn()
    const event: RealtimeEvent = {
      type: 'task.updated',
      board_id: activeBoard.id,
      task_id: 'task-1',
      payload: {
        task: { id: 'task-1', board_id: activeBoard.id, status_id: 'todo', title: 'Existing task', assignee_id: null },
      },
    }

    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast })

    expect(activeBoard.tasks[0].assignee_id).toBeNull()
    expect(showToast).toHaveBeenCalledWith('Task unassigned', expect.objectContaining({ tone: 'info', message: expect.stringContaining('Existing task') }))
    expect(showToast).not.toHaveBeenCalledWith('Task assigned', expect.anything())
  })

  it('keeps the generic toast when task.updated does not change the assignee', () => {
    const activeBoard = board()
    const showToast = vi.fn()
    const event: RealtimeEvent = {
      type: 'task.updated',
      board_id: activeBoard.id,
      task_id: 'task-1',
      payload: {
        task: { id: 'task-1', board_id: activeBoard.id, status_id: 'todo', title: 'Existing task', description: 'Updated body', assignee_id: null },
      },
    }

    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast })

    expect(showToast).toHaveBeenCalledWith('Task updated', expect.objectContaining({ tone: 'info' }))
  })

  it('updates comments only while their board is active', () => {
    const activeBoard = board()
    const showToast = vi.fn()
    const event: RealtimeEvent = {
      type: 'comment.created',
      board_id: activeBoard.id,
      task_id: 'task-1',
      payload: { comment: { id: 'comment-1', body: 'Ready' } },
    }

    handleRealtimeEvent(event, { activeBoardId: '', board: activeBoard, showToast })
    expect(activeBoard.tasks[0].comments).toEqual([])

    handleRealtimeEvent(event, { activeBoardId: activeBoard.id, board: activeBoard, showToast })
    expect(activeBoard.tasks[0].comments).toEqual([{ id: 'comment-1', body: 'Ready' }])
    expect(showToast).toHaveBeenCalledTimes(2)
  })
})
