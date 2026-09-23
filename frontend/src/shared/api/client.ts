// JSON-клиент для backend API. Ошибки сервера превращаются в Error с полями
// status и field; 401 на защищённых запросах сообщает приложению об истёкшей сессии.
type ApiError = Error & { field?: string; status?: number }

export async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options?.headers ?? {}) },
    ...options,
  })
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { error?: { message?: string; field?: string } }
    const error = new Error(body.error?.message ?? 'Не удалось выполнить запрос') as ApiError
    error.field = body.error?.field
    error.status = response.status
    Object.assign(error, body)
    // A protected request can discover an expired/revoked session after the
    // initial /me check. Notify the shell so it returns to the login screen
    // instead of leaving the employee on a broken protected page. Auth
    // endpoints themselves are excluded so an ordinary bad login stays in
    // the compact form with its generic error message.
    if (response.status === 401 && !path.startsWith('/api/auth/')) {
      window.dispatchEvent(new Event('tradeflow-session-expired'))
    }
    throw error
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
