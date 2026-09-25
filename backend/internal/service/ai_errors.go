package service

// Errors of the AI provider (DeepSeek) and their user-facing messages.

import "errors"

var (
	ErrAINotConfigured    = errors.New("ai provider is not configured")
	ErrAIUnauthorized     = errors.New("ai provider authorization failed")
	ErrAIRateLimited      = errors.New("ai provider rate limit")
	ErrAITimeout          = errors.New("ai provider timeout")
	ErrAIRequestRejected  = errors.New("ai provider rejected the request")
	ErrAIModelUnavailable = errors.New("ai model unavailable")
	ErrAIBadResponse      = errors.New("ai provider returned invalid response")
	ErrAIRequestTooLarge  = errors.New("ai input is too large")
	ErrAIUnavailable      = errors.New("ai service unavailable")
)

func AIErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrAINotConfigured):
		return "Ключ ИИ-провайдера не настроен на сервере"
	case errors.Is(err, ErrAIUnauthorized):
		return "ИИ-провайдер отклонил ключ API"
	case errors.Is(err, ErrAIRateLimited):
		return "ИИ-провайдер временно ограничил запросы. Повторите позже"
	case errors.Is(err, ErrAITimeout):
		return "ИИ-провайдер не ответил вовремя. Повторите запрос"
	case errors.Is(err, ErrAIRequestRejected):
		return "ИИ-провайдер отклонил запрос. Проверьте модель и формат запроса"
	case errors.Is(err, ErrAIModelUnavailable):
		return "Выбранная модель ИИ-провайдера недоступна для этого ключа"
	case errors.Is(err, ErrAIRequestTooLarge):
		return "Запрос к ИИ слишком большой"
	case errors.Is(err, ErrAIBadResponse):
		return "ИИ-провайдер вернул ответ, который не прошёл проверку"
	case errors.Is(err, ErrAIUnavailable):
		return "ИИ-провайдер временно недоступен. Повторите запрос"
	default:
		return "Не удалось получить ответ ИИ"
	}
}
