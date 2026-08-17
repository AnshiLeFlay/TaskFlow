import Keycloak from 'keycloak-js'

export const keycloak = new Keycloak({
  url: import.meta.env.VITE_KEYCLOAK_URL || 'http://localhost:8082',
  realm: import.meta.env.VITE_KEYCLOAK_REALM || 'taskflow',
  clientId: import.meta.env.VITE_KEYCLOAK_CLIENT_ID || 'taskflow-web',
})

let initialized: Promise<boolean> | undefined

export function initKeycloak(): Promise<boolean> {
  if (!initialized) {
    initialized = keycloak.init({
      onLoad: 'check-sso',
      pkceMethod: 'S256',
      checkLoginIframe: false,
    }).catch((error) => {
      initialized = undefined
      throw error
    })
  }
  return initialized
}

export async function validToken(): Promise<string | undefined> {
  if (!keycloak.authenticated) return undefined
  try {
    await keycloak.updateToken(30)
  } catch {
    await keycloak.logout({ redirectUri: `${window.location.origin}/login` })
    return undefined
  }
  return keycloak.token
}
