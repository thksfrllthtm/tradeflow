// Текст ошибки для баннера: сообщение сервера/исключения или общий запасной текст.
export function errorMessage(reason: unknown) {
  return reason instanceof Error ? reason.message : 'Не удалось выполнить действие'
}
