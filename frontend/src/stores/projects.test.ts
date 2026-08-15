import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { taskflowApi } from '../api/taskflow'
import type { ProjectMember } from '../domain/types'
import { useProjectsStore } from './projects'

vi.mock('../api/taskflow', () => ({
  taskflowApi: {
    addMember: vi.fn(),
    listMembers: vi.fn(),
  },
}))

const addMemberMock = vi.mocked(taskflowApi.addMember)
const listMembersMock = vi.mocked(taskflowApi.listMembers)

describe('projects store: member cache', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    addMemberMock.mockReset()
    listMembersMock.mockReset()
  })

  it('does not poison the cache when addMember runs before members were ever fetched: a later loadMembers still does the real fetch and serves the full list', async () => {
    const newMember: ProjectMember = { user_id: 'user-2', role: 'member' }
    addMemberMock.mockResolvedValue(newMember)
    const fullList: ProjectMember[] = [{ user_id: 'user-1', role: 'admin' }, newMember]
    listMembersMock.mockResolvedValue(fullList)

    const store = useProjectsStore()
    expect(store.membersByProject['project-1']).toBeUndefined()

    // Natural ordering: a member is added (e.g. from ProjectsView, which never calls
    // loadMembers) before this session has ever fetched the member list for this project.
    await store.addMember('project-1', 'user-2', 'member')

    const result = await store.loadMembers('project-1')

    expect(listMembersMock).toHaveBeenCalledTimes(1)
    expect(result).toEqual(fullList)
    expect(store.membersByProject['project-1']).toEqual(fullList)
  })

  it('keeps an already-populated cache in sync immediately, without waiting for a reload', async () => {
    // Pass a fresh array literal to the mock (rather than a shared reference) so this
    // test's own "before" snapshot below isn't mutated by the store's later push.
    listMembersMock.mockResolvedValue([{ user_id: 'user-1', role: 'admin' }])
    const newMember: ProjectMember = { user_id: 'user-2', role: 'member' }
    addMemberMock.mockResolvedValue(newMember)

    const store = useProjectsStore()
    await store.loadMembers('project-1')
    expect(store.membersByProject['project-1']).toEqual([{ user_id: 'user-1', role: 'admin' }])

    await store.addMember('project-1', 'user-2', 'member')

    expect(store.membersByProject['project-1']).toEqual([{ user_id: 'user-1', role: 'admin' }, newMember])
    // The cache was already populated, so a subsequent loadMembers should serve it from
    // cache rather than re-fetching.
    await store.loadMembers('project-1')
    expect(listMembersMock).toHaveBeenCalledTimes(1)
  })
})
