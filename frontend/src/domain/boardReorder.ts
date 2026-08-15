export interface StatusPosition {
  id: string
  position: number
}

/**
 * Plans the network calls needed to swap two board statuses' positions
 * without ever colliding with the backend's unique (board_id, position)
 * index while the swap is in flight.
 *
 * A naive two-call swap (first -> second's position, second -> first's
 * position) always fails on the first call: at that instant `second` still
 * occupies the position `first` is being moved to. Instead this parks
 * `first` at a position beyond every status on the board, moves `second`
 * into `first`'s old slot, then moves the parked `first` into `second`'s
 * old slot — three calls, each collision-free against the current state of
 * the board.
 *
 * Returns an empty plan if either id is unknown or they're the same status.
 */
export function planStatusSwap(statuses: StatusPosition[], firstId: string, secondId: string): StatusPosition[] {
  const first = statuses.find((status) => status.id === firstId)
  const second = statuses.find((status) => status.id === secondId)
  if (!first || !second || first.id === second.id) return []
  const parkPosition = Math.max(...statuses.map((status) => status.position)) + 1
  return [
    { id: first.id, position: parkPosition },
    { id: second.id, position: first.position },
    { id: first.id, position: second.position },
  ]
}
