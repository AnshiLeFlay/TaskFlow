import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, RouterView, type Router } from 'vue-router'
import { taskflowApi } from '../api/taskflow'
import type { Board, Project } from '../domain/types'
import { useToastStore } from '../stores/toasts'
import BoardView from './BoardView.vue'

vi.mock('../api/taskflow', () => ({
  taskflowApi: {
    projects: vi.fn(),
    boards: vi.fn(),
    board: vi.fn(),
    listMembers: vi.fn(),
    addComment: vi.fn(),
    updateTask: vi.fn(),
    transition: vi.fn(),
    createTask: vi.fn(),
    updateStatus: vi.fn(),
  },
}))

const projectsMock = vi.mocked(taskflowApi.projects)
const boardsMock = vi.mocked(taskflowApi.boards)
const boardMock = vi.mocked(taskflowApi.board)
const listMembersMock = vi.mocked(taskflowApi.listMembers)
const updateTaskMock = vi.mocked(taskflowApi.updateTask)
const transitionMock = vi.mocked(taskflowApi.transition)
const addCommentMock = vi.mocked(taskflowApi.addComment)
const updateStatusMock = vi.mocked(taskflowApi.updateStatus)

function seedProject(): Project {
  return { id: 'project-1', name: 'Project', role: 'admin' }
}

function seedBoard(): Board {
  return {
    id: 'board-1',
    project_id: 'project-1',
    name: 'Board',
    statuses: [{ id: 'todo', name: 'To Do', position: 0 }, { id: 'done', name: 'Done', position: 1 }],
    tasks: [{ id: 'task-1', board_id: 'board-1', status_id: 'todo', title: 'Ship it', comments: [] }],
    rules: [],
  }
}

async function mountBoard(): Promise<{ wrapper: ReturnType<typeof mount>; router: Router }> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/projects/:projectId/boards/:boardId', name: 'board', component: BoardView },
      { path: '/projects', name: 'projects', component: { template: '<div />' } },
    ],
  })
  await router.push('/projects/project-1/boards/board-1')
  await router.isReady()

  const pinia = createPinia()
  setActivePinia(pinia)

  const wrapper = mount(RouterView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}

describe('BoardView: blocked-transition save flow', () => {
  beforeEach(() => {
    projectsMock.mockReset().mockResolvedValue([seedProject()])
    boardsMock.mockReset().mockResolvedValue([{ id: 'board-1', project_id: 'project-1', name: 'Board', statuses: [], tasks: [], rules: [] }])
    boardMock.mockReset().mockResolvedValue(seedBoard())
    listMembersMock.mockReset().mockResolvedValue([])
    updateTaskMock.mockReset()
    transitionMock.mockReset()
    addCommentMock.mockReset()
    updateStatusMock.mockReset()
  })

  it('leaves the modal open, reverts the status select, and shows the partial-save toast when the transition is blocked', async () => {
    updateTaskMock.mockResolvedValue({ id: 'task-1', status_id: 'todo', title: 'Ship it v2' })
    transitionMock.mockRejectedValue(new Error('A comment is required before this transition.'))

    const { wrapper } = await mountBoard()

    await wrapper.get('[data-testid="task-card"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="task-modal"]').exists()).toBe(true)

    await wrapper.get('[data-testid="task-title"]').setValue('Ship it v2')
    await wrapper.get('[data-testid="task-status"]').setValue('done')
    await wrapper.get('[data-testid="task-modal"]').trigger('submit')
    await flushPromises()

    // The core assertion the review asked for: a blocked transition must NOT close the modal.
    expect(wrapper.find('[data-testid="task-modal"]').exists()).toBe(true)
    // The forced remount (:key bump) re-derives the status select from the task's real,
    // store-reverted status_id, not the rejected target.
    expect((wrapper.get('[data-testid="task-status"]').element as HTMLSelectElement).value).toBe('todo')

    const toasts = useToastStore()
    expect(toasts.items.some((item) => item.title === 'Status change blocked'
      && item.message?.includes('A comment is required before this transition.'))).toBe(true)
  })

  it('closes the modal only on a full save success', async () => {
    updateTaskMock.mockResolvedValue({ id: 'task-1', status_id: 'todo', title: 'Ship it v2' })

    const { wrapper } = await mountBoard()

    await wrapper.get('[data-testid="task-card"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="task-modal"]').exists()).toBe(true)

    // No status change in this save, so the transition step should not even be attempted.
    await wrapper.get('[data-testid="task-title"]').setValue('Ship it v2')
    await wrapper.get('[data-testid="task-modal"]').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[data-testid="task-modal"]').exists()).toBe(false)
    expect(transitionMock).not.toHaveBeenCalled()
  })
})

describe('BoardView: column reorder', () => {
  beforeEach(() => {
    projectsMock.mockReset().mockResolvedValue([seedProject()])
    boardsMock.mockReset().mockResolvedValue([{ id: 'board-1', project_id: 'project-1', name: 'Board', statuses: [], tasks: [], rules: [] }])
    boardMock.mockReset().mockResolvedValue(seedBoard())
    listMembersMock.mockReset().mockResolvedValue([])
    updateStatusMock.mockReset()
  })

  it('swaps two columns with a park-then-move sequence instead of a two-call swap that always fails', async () => {
    updateStatusMock.mockImplementation(async (id: string, input) => ({ id, name: id === 'todo' ? 'To Do' : 'Done', position: input.position ?? 0 }))

    const { wrapper } = await mountBoard()

    await wrapper.get('[data-testid="board-settings-button"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="board-settings"]').exists()).toBe(true)

    // seedBoard has todo (position 0) and done (position 1); moving todo right swaps it with done.
    await wrapper.get('[aria-label="Move column right"]').trigger('click')
    await flushPromises()

    // Regression coverage: a naive two-call swap (todo -> 1, done -> 0) fails immediately
    // because `done` still occupies position 1 when the first call lands under the
    // backend's unique (board_id, position) index. The fix parks `todo` beyond the
    // board's highest position first.
    expect(updateStatusMock.mock.calls.map(([id, input]) => [id, input.position])).toEqual([
      ['todo', 2],
      ['done', 0],
      ['todo', 1],
    ])
  })

  it('shows an error toast and reloads the board when a reorder call fails', async () => {
    updateStatusMock.mockRejectedValue(new Error('position already used'))

    const { wrapper } = await mountBoard()
    boardMock.mockClear()

    await wrapper.get('[data-testid="board-settings-button"]').trigger('click')
    await flushPromises()
    await wrapper.get('[aria-label="Move column right"]').trigger('click')
    await flushPromises()

    const toasts = useToastStore()
    expect(toasts.items.some((item) => item.title === 'Could not reorder columns')).toBe(true)
    expect(boardMock).toHaveBeenCalled()
  })
})
