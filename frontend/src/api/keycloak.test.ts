import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  init: vi.fn().mockResolvedValue(false),
}))

vi.mock('keycloak-js', () => ({
  default: class MockKeycloak {
    authenticated = false
    init = mocks.init
  },
}))

import { initKeycloak } from './keycloak'

describe('Keycloak initialization', () => {
  beforeEach(() => {
    mocks.init.mockClear()
  })

  it('uses first-party check-sso without a third-party cookie iframe', async () => {
    await expect(initKeycloak()).resolves.toBe(false)

    expect(mocks.init).toHaveBeenCalledWith({
      onLoad: 'check-sso',
      pkceMethod: 'S256',
      checkLoginIframe: false,
    })
    expect(mocks.init.mock.calls[0][0]).not.toHaveProperty('silentCheckSsoRedirectUri')
  })
})
