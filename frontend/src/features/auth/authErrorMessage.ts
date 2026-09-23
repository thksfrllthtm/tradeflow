// Сообщение для форм входа/регистрации: сетевая ошибка отдельно от ответа сервера.
export function authErrorMessage(reason: unknown, fallback: string) {
  const typed = reason as Error & { status?: number }
  if (typed?.status === undefined) return 'Сетевая ошибка. Проверьте подключение.'
  return typed.message || fallback
}
