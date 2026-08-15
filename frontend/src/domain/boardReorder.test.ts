import { describe, expect, it } from 'vitest'
import { planStatusSwap } from './boardReorder'

describe('planStatusSwap', () => {
  it('parks the first status beyond the highest position, then swaps into the vacated slots', () => {
    const statuses = [
      { id: 'todo', position: 0 },
      { id: 'doing', position: 1 },
      { id: 'done', position: 2 },
    ]

    const steps = planStatusSwap(statuses, 'todo', 'doing')

    expect(steps).toEqual([
      { id: 'todo', position: 3 }, // parked beyond max(0,1,2)=2
      { id: 'doing', position: 0 }, // moves into todo's old slot
      { id: 'todo', position: 1 }, // moves into doing's old slot
    ])
  })

  it('never asks to move a status into a position another status currently occupies', () => {
    const statuses = [
      { id: 'todo', position: 0 },
      { id: 'doing', position: 1 },
      { id: 'done', position: 2 },
    ]
    const occupied = new Map(statuses.map((status) => [status.position, status.id]))

    const steps = planStatusSwap(statuses, 'todo', 'done')

    for (const step of steps) {
      const currentOccupant = occupied.get(step.position)
      if (currentOccupant !== undefined) expect(currentOccupant).toBe(step.id)
      // Moving `step.id` frees up whatever position it previously held.
      for (const [position, id] of occupied) if (id === step.id) occupied.delete(position)
      occupied.set(step.position, step.id)
    }
  })

  it('is symmetric regardless of call order', () => {
    const statuses = [
      { id: 'a', position: 0 },
      { id: 'b', position: 1 },
    ]
    expect(planStatusSwap(statuses, 'a', 'b')).toEqual([
      { id: 'a', position: 2 },
      { id: 'b', position: 0 },
      { id: 'a', position: 1 },
    ])
    expect(planStatusSwap(statuses, 'b', 'a')).toEqual([
      { id: 'b', position: 2 },
      { id: 'a', position: 1 },
      { id: 'b', position: 0 },
    ])
  })

  it('returns an empty plan for unknown ids or a no-op swap with itself', () => {
    const statuses = [{ id: 'a', position: 0 }, { id: 'b', position: 1 }]
    expect(planStatusSwap(statuses, 'a', 'missing')).toEqual([])
    expect(planStatusSwap(statuses, 'a', 'a')).toEqual([])
  })
})
