import type { Board, Comment, RealtimeEvent, Task } from '../domain/types'
import type { ToastTone } from '../stores/toasts'

type SupportedEventType = 'task.transitioned' | 'task.updated' | 'comment.created'

export interface RealtimeEventContext {
  activeBoardId: string
  board: Board | null
  showToast: (title: string, options: { message?: string; tone?: ToastTone }) => void
  refreshBoard?: (boardId: string) => void
  /** The signed-in user's profile id (useAuthStore().user?.id), used to distinguish "you were assigned". */
  currentUserId?: string
}

function eventType(event: RealtimeEvent): SupportedEventType | undefined {
  const value = event.type || event.event_type
  return value === 'task.transitioned' || value === 'task.updated' || value === 'comment.created' ? value : undefined
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value ? value : undefined
}

/**
 * Shows membership-wide notifications, while applying payloads only to the board
 * currently visible in the router. This prevents a global stream from leaking a
 * task from one board into another board's Pinia state.
 */
export function handleRealtimeEvent(event: RealtimeEvent, context: RealtimeEventContext): void {
  const type = eventType(event)
  if (!type) return

  const payload = event.payload || {}
  const embeddedTask = (event.task || payload.task) as Task | undefined
  const eventBoardId = event.board_id
    || stringValue(payload.board_id)
    || embeddedTask?.board_id
    || ''
  const isActiveBoard = Boolean(
    context.activeBoardId
    && eventBoardId === context.activeBoardId
    && context.board?.id === context.activeBoardId,
  )

  // Set only when a task.updated event changes the assignee of a task we already had
  // state for (i.e. there is a "previous" assignee to compare against). newAssigneeId is
  // undefined for an unassignment (assignee_id cleared), as opposed to for an assignment.
  let assignmentChange: { title: string; newAssigneeId?: string } | undefined

  if (isActiveBoard && context.board) {
    if (type === 'task.transitioned' || type === 'task.updated') {
      const taskBelongsToBoard = embeddedTask && (!embeddedTask.board_id || embeddedTask.board_id === context.activeBoardId)
      if (taskBelongsToBoard) {
        const current = context.board.tasks.find((task) => task.id === embeddedTask.id)
        if (type === 'task.updated' && current) {
          const previousAssigneeId = current.assignee_id || undefined
          const nextAssigneeId = embeddedTask.assignee_id || undefined
          if (previousAssigneeId !== nextAssigneeId) {
            assignmentChange = { title: embeddedTask.title || current.title, newAssigneeId: nextAssigneeId }
          }
        }
        if (current) Object.assign(current, embeddedTask)
        else context.board.tasks.push(embeddedTask)
      } else if (!embeddedTask) {
        context.refreshBoard?.(context.activeBoardId)
      }
    } else {
      const task = context.board.tasks.find((item) => item.id === event.task_id)
      const comment = payload.comment as Comment | undefined
      if (task && comment && !task.comments?.some((item) => item.id === comment.id)) {
        (task.comments ||= []).push(comment)
      } else if (!comment) {
        context.refreshBoard?.(context.activeBoardId)
      }
    }
  }

  const taskTitle = embeddedTask?.title || stringValue(payload.task_title) || stringValue(payload.title) || 'A task'
  if (type === 'task.transitioned') {
    context.showToast('Task status changed', { tone: 'info', message: event.message || `${taskTitle} moved to a new column.` })
  } else if (type === 'comment.created') {
    context.showToast('New comment', { tone: 'info', message: event.message || `${taskTitle} has a new comment.` })
  } else if (assignmentChange && !assignmentChange.newAssigneeId) {
    context.showToast('Task unassigned', { tone: 'info', message: `${assignmentChange.title} no longer has an assignee.` })
  } else if (assignmentChange) {
    const assignedToYou = Boolean(
      context.currentUserId && assignmentChange.newAssigneeId === context.currentUserId,
    )
    context.showToast(assignedToYou ? 'You were assigned' : 'Task assigned', {
      tone: 'info',
      message: assignedToYou ? `You were assigned to ${assignmentChange.title}.` : `${assignmentChange.title} was assigned to a new teammate.`,
    })
  } else {
    context.showToast('Task updated', { tone: 'info', message: event.message || `${taskTitle} details or assignment changed.` })
  }
}
