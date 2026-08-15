import { expect, test } from '@playwright/test'
import { adminCredentials, column, configureBoard, createProjectAndBoard, createTask, login, moveTask, register, uniqueName } from './helpers'

test.describe('TaskFlow acceptance flows', () => {
  test('registration, project, board and comment-gated workflow', async ({ page }) => {
    await register(page)
    const projectName = uniqueName('Launch')
    const boardName = uniqueName('Delivery')
    const taskTitle = uniqueName('Prepare release')

    await createProjectAndBoard(page, projectName, boardName)
    await configureBoard(page)
    await createTask(page, taskTitle, 'To Do')

    await moveTask(page, taskTitle, 'In Progress')
    await expect(page.getByTestId('toast').filter({ hasText: 'Transition blocked' })).toBeVisible()
    await expect(column(page, 'To Do').getByTestId('task-card').filter({ hasText: taskTitle })).toBeVisible()

    await page.getByTestId('task-card').filter({ hasText: taskTitle }).click()
    await page.getByTestId('task-comment').fill('Acceptance criteria are verified.')
    await page.getByTestId('task-modal').getByRole('button', { name: 'Save changes' }).click()
    await expect(page.getByTestId('task-modal')).toBeHidden()

    await moveTask(page, taskTitle, 'In Progress')
    await expect(page.getByTestId('toast').filter({ hasText: 'Task moved' })).toBeVisible()
    await expect(column(page, 'In Progress').getByTestId('task-card').filter({ hasText: taskTitle })).toBeVisible()

    await page.reload()
    const persistedTask = column(page, 'In Progress').getByTestId('task-card').filter({ hasText: taskTitle })
    await expect(persistedTask).toBeVisible()
    await persistedTask.click()
    await expect(page.getByTestId('task-modal').getByText('Acceptance criteria are verified.')).toBeVisible()
  })

  test('a transition in one browser produces a realtime toast in another', async ({ browser }) => {
    const firstContext = await browser.newContext()
    const first = await firstContext.newPage()
    await login(first, adminCredentials)

    const projectName = uniqueName('Realtime')
    const boardName = uniqueName('Live board')
    const taskTitle = uniqueName('Broadcast update')
    await createProjectAndBoard(first, projectName, boardName)
    await configureBoard(first, 'To Do', 'Done', false)
    await createTask(first, taskTitle, 'To Do')

    const secondContext = await browser.newContext()
    const second = await secondContext.newPage()
    const socketReady = second
      .waitForEvent('websocket', { predicate: (socket) => socket.url().includes('/ws'), timeout: 15_000 })
      .then((socket) => socket.waitForEvent('framereceived', {
        predicate: ({ payload }) => String(payload).includes('"type":"connected"'),
        timeout: 15_000,
      }))
    await login(second, adminCredentials)
    await socketReady
    await expect.poll(() => new URL(second.url()).pathname).toBe('/projects')

    await moveTask(first, taskTitle, 'Done')
    await expect(second.getByTestId('toast').filter({ hasText: /Task status changed/ })).toBeVisible({ timeout: 15_000 })

    await secondContext.close()
    await firstContext.close()
  })
})
