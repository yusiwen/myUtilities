const BASE = 'api'

async function request(url, options = {}) {
  const res = await fetch(BASE + url, {
    headers: { 'Content-Type': 'application/json', ...options.headers },
    ...options,
  })
  if (!res.ok) {
    const text = await res.text()
    let msg
    try { msg = JSON.parse(text).error || text } catch { msg = text }
    throw new Error(msg)
  }
  if (res.status === 204) return null
  return res.json()
}

export async function listEndpoints() {
  return request('/endpoints')
}

export async function createEndpoint(data) {
  return request('/endpoints', { method: 'POST', body: JSON.stringify(data) })
}

export async function updateEndpoint(id, data) {
  return request(`/endpoints/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

export async function deleteEndpoint(id) {
  return request(`/endpoints/${id}`, { method: 'DELETE' })
}

export async function saveToConfig() {
  return request('/save', { method: 'POST' })
}

export async function listLogs() {
  return request('/logs')
}
