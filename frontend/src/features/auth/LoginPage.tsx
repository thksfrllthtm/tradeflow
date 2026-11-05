import { FormEvent, useState } from 'react'
import { User } from '@/shared/types/user'
import { authApi } from './authApi'
import { authErrorMessage } from './authErrorMessage'
import { AuthRates } from './AuthRates'
import './auth.css'

export function LoginPage({ notice, onLogin, onRegister }: { notice: string; onLogin: (user: User) => void; onRegister: () => void }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (loading) return
    setError('')
    setLoading(true)
    try {
      onLogin(await authApi.login(email, password))
    } catch (reason) {
      setError(authErrorMessage(reason, 'Не удалось войти'))
    } finally {
      setLoading(false)
    }
  }

  return <main className="login-shell">
    <section className="login-card">
      <p className="eyebrow">TRADEFLOW</p>
      <h1>Вход</h1>
      <p className="muted">Внутренний калькулятор для сотрудников</p>
      {notice && <p className="auth-notice" role="status">{notice}</p>}
      <form onSubmit={submit} className="form-stack">
        <label>Рабочий email<input type="email" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>
        <label>Пароль<span className="password-field"><input type={showPassword ? 'text' : 'password'} autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} required /><button type="button" className="password-toggle" onClick={() => setShowPassword((visible) => !visible)} aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'}>{showPassword ? 'Скрыть' : 'Показать'}</button></span></label>
        {error && <p className="error" role="alert">{error}</p>}
        <button type="submit" disabled={loading}>{loading ? 'Входим…' : 'Войти'}</button>
      </form>
      <button type="button" className="secondary auth-switch" onClick={onRegister}>Нет аккаунта? Зарегистрироваться</button>
      <AuthRates />
    </section>
  </main>
}
