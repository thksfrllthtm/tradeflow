// Понятный текст ошибки запроса калькулятора: сеть, отклонённые данные (400), недоступный сервис (5xx).
export function requestErrorMessage(reason: unknown) {
  const error = reason as { message?: unknown; status?: number }
  const rawMessage = typeof error.message === 'string' ? error.message : ''
  if (reason instanceof TypeError && /fetch|network|load failed/i.test(reason.message)) {
    return 'Не удалось связаться с сервером. Проверьте, что backend запущен, и повторите действие.'
  }
  if (error.status === 400) {
    return rawMessage && rawMessage !== 'Не удалось выполнить запрос'
      ? `Сервер отклонил данные: ${rawMessage}`
      : 'Сервер отклонил данные. Проверьте обязательные поля и карточку ТН ВЭД в строках.'
  }
  if (error.status === 502 || error.status === 503 || error.status === 504) {
    return 'Сервис временно недоступен. Проверьте backend и повторите действие через несколько секунд.'
  }
  return rawMessage || 'Не удалось выполнить действие'
}
