import { FormEvent, useState } from 'react'
import { User } from '@/shared/types/user'
import { authApi } from './authApi'
import { authErrorMessage } from './authErrorMessage'
import { AuthRates } from './AuthRates'
import './auth.css'

export function RegisterPage({ onLogin, onLoginMode }: { onLogin: (user: User) => void; onLoginMode: () => void }) {
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [fieldError, setFieldError] = useState('')
  const [loading, setLoading] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (loading) return
    setError(''); setFieldError(''); setLoading(true)
    try {
      onLogin(await authApi.register(name, email, password))
    } catch (reason) {
      const typed = reason as Error & { field?: string }
      if (typed.field) setFieldError(typed.message)
      else setError(authErrorMessage(reason, 'Не удалось создать аккаунт'))
    } finally { setLoading(false) }
  }

  return <main className="login-shell">
    <section className="login-card">
      <p className="eyebrow">TRADEFLOW</p>
      <h1>Создать аккаунт</h1>
      <p className="muted">Обычный аккаунт сотрудника с доступом только к своим сметам.</p>
      <form onSubmit={submit} className="form-stack">
        <label>Имя<input type="text" autoComplete="name" value={name} onChange={(event) => setName(event.target.value)} maxLength={120} required /></label>
        <label>Email<input type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>
        <label>Пароль<span className="password-field"><input type={showPassword ? 'text' : 'password'} autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} required /><button type="button" className="password-toggle" onClick={() => setShowPassword((visible) => !visible)} aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'}>{showPassword ? 'Скрыть' : 'Показать'}</button></span><small className="field-hint">Минимум 8 символов. Пробелы и длинные пароли разрешены.</small></label>
        {fieldError && <p className="error" role="alert">{fieldError}</p>}
        {error && <p className="error" role="alert">{error}</p>}
        <button type="submit" disabled={loading}>{loading ? 'Создаём…' : 'Создать аккаунт'}</button>
      </form>
      <button type="button" className="auth-switch" onClick={onLoginMode}>Уже есть аккаунт? Войти</button>
      <AuthRates />
    </section>
  </main>
}
