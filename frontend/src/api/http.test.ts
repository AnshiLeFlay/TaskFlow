import { describe, expect, it } from 'vitest'
import { unwrapItem, unwrapList } from './http'

describe('API response helpers', () => {
  it('accepts wrapped and bare lists', () => {
    expect(unwrapList<{ id: string }>({ projects: [{ id: 'one' }] }, 'projects')).toEqual([{ id: 'one' }])
    expect(unwrapList([{ id: 'two' }], 'projects')).toEqual([{ id: 'two' }])
  })

  it('accepts wrapped and bare resources', () => {
    expect(unwrapItem<{ id: string }>({ board: { id: 'board-1' } }, 'board')).toEqual({ id: 'board-1' })
    expect(unwrapItem({ id: 'board-2' }, 'board')).toEqual({ id: 'board-2' })
  })
})
