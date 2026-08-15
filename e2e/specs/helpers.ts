import { expect, type Page } from '@playwright/test'

export interface Credentials { username: string; password: string }

export const adminCredentials: Credentials = {
  username: process.env.E2E_ADMIN_USERNAME || 'alice',
  password: process.env.E2E_ADMIN_PASSWORD || 'alice',
}

export function uniqueName(prefix: string): string {
  return `${prefix} ${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
}

export async function login(page: Page, credentials: Credentials = adminCredentials) {
  await page.goto('/login')
  const loginButton = page.getByTestId('login-button')
  await expect(loginButton).toBeVisible()
  await loginButton.click()
  await page.locator('#username, input[name="username"]').first().fill(credentials.username)
  await page.locator('#password, input[name="password"]').first().fill(credentials.password)
  await page.locator('#kc-login, input[name="login"], button[type="submit"]').first().click()
  await expect.poll(() => new URL(page.url()).pathname).toBe('/projects')
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
}

export async function register(page: Page): Promise<Credentials> {
  const suffix = `${Date.now()}${Math.random().toString(36).slice(2, 6)}`
  const credentials = { username: `flow_${suffix}`, password: `Flow!${suffix}Aa9` }
  await page.goto('/login')
  await page.getByTestId('register-button').click()
  await page.locator('#firstName, input[name="firstName"]').fill('Flow')
  await page.locator('#lastName, input[name="lastName"]').fill('Tester')
  await page.locator('#email, input[name="email"]').fill(`${credentials.username}@example.test`)
  await page.locator('#username, input[name="username"]').fill(credentials.username)
  await page.locator('#password, input[name="password"]').fill(credentials.password)
  await page.locator('#password-confirm, input[name="password-confirm"]').fill(credentials.password)
  await page.locator('#kc-register, input[type="submit"], button[type="submit"]').last().click()
  await expect.poll(() => new URL(page.url()).pathname, { timeout: 25_000 }).toBe('/projects')
  await expect(page.getByRole('heading', { name: 'Projects' })).toBeVisible()
  return credentials
}

export async function createProjectAndBoard(page: Page, projectName: string, boardName: string): Promise<string> {
  await page.getByTestId('new-project').click()
  await page.getByTestId('project-name').fill(projectName)
  await page.getByTestId('project-modal').getByRole('button', { name: 'Create project' }).click()
  await expect(page.getByText(projectName, { exact: true }).first()).toBeVisible()
  await page.getByText(projectName, { exact: true }).first().click()
  await page.getByRole('button', { name: '+ Board' }).click()
  await page.getByTestId('board-name').fill(boardName)
  await page.getByTestId('board-modal').getByRole('button', { name: 'Create board' }).click()
  await expect(page).toHaveURL(/\/projects\/[^/]+\/boards\/[^/]+$/)
  return page.url()
}

export async function addStatus(page: Page, name: string) {
  await page.getByTestId('status-name').fill(name)
  await page.getByTestId('status-form').getByRole('button', { name: 'Add' }).click()
  await expect(page.getByTestId('board-settings').getByText(name, { exact: true })).toBeVisible()
}

export async function configureBoard(page: Page, from = 'To Do', to = 'In Progress', requiresComment = true) {
  await page.getByTestId('board-settings-button').click()
  const settings = page.getByTestId('board-settings')
  if (!await settings.getByText(from, { exact: true }).isVisible().catch(() => false)) await addStatus(page, from)
  if (!await settings.getByText(to, { exact: true }).isVisible().catch(() => false)) await addStatus(page, to)
  await page.getByRole('button', { name: 'Workflow rules' }).click()
  await page.getByTestId('rule-from').selectOption({ label: from })
  await page.getByTestId('rule-to').selectOption({ label: to })
  await page.getByTestId('rule-condition').selectOption(requiresComment ? 'requires_comment' : 'author_only')
  await page.getByTestId('rule-form').getByRole('button', { name: 'Add workflow rule' }).click()
  await expect(page.getByTestId('workflow-rule')).toContainText(`${from}`)
  await page.getByRole('button', { name: 'Close settings' }).click()
  await expect(page.getByTestId('kanban-board')).toBeVisible()
}

export async function createTask(page: Page, title: string, status: string) {
  await page.getByTestId('new-task').click()
  await page.getByTestId('task-title').fill(title)
  await page.getByTestId('task-status').selectOption({ label: status })
  await page.getByTestId('task-modal').getByRole('button', { name: 'Create task' }).click()
  await expect(page.getByTestId('task-card').filter({ hasText: title })).toBeVisible()
}

export function column(page: Page, name: string) {
  return page.getByTestId('kanban-column').filter({ has: page.getByRole('heading', { name, exact: true }) })
}

export async function moveTask(page: Page, title: string, targetColumn: string) {
  await page.getByTestId('task-card').filter({ hasText: title }).dragTo(column(page, targetColumn))
}
