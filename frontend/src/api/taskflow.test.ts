import { afterEach, describe, expect, it, vi } from 'vitest'
import { taskflowApi } from './taskflow'

describe('taskflowApi.listMembers', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('requests the project members endpoint and unwraps the envelope', async () => {
    const members = [{ user_id: 'user-1', role: 'admin' }, { user_id: 'user-2', role: 'member' }]
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify({ members }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const result = await taskflowApi.listMembers('project-1')

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(String(fetchMock.mock.calls[0][0])).toContain('/projects/project-1/members')
    expect(result).toEqual(members)
  })

  it('also accepts a bare array response', async () => {
    const members = [{ user_id: 'user-1', role: 'viewer' }]
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => JSON.stringify(members),
    })
    vi.stubGlobal('fetch', fetchMock)

    const result = await taskflowApi.listMembers('project-1')

    expect(result).toEqual(members)
  })
})
