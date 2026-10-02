import { FormEvent, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { Role, roleLabels, User } from '@/shared/types/user'
import { EmployeeEstimatesPanel } from './EmployeeEstimatesPanel'
import { Employee, employeesApi, EmployeeUpdate } from './employeesApi'
import '@/shared/styles/list-page.css'
import '@/shared/styles/verification-status.css'
import './employees.css'

const roles = Object.keys(roleLabels) as Role[]
const emptyForm = { name: '', email: '', password: '', role: 'invoicer' as Role }
const estimatesWord = (count: number) => plural(count, 'смета', 'сметы', 'смет')

// Сотрудники: роли, данные, пароль, блокировка, удаление и сметы каждого. Страница доступна
// только администратору: сервер проверяет права сам, а приложение не показывает её остальным.
export function EmployeesPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [items, setItems] = useState<Employee[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [saving, setSaving] = useState(false)
  const [passwordFor, setPasswordFor] = useState<number | null>(null)
  const [newPassword, setNewPassword] = useState('')
  const [editing, setEditing] = useState<{ id: number; name: string; email: string } | null>(null)
  const [estimatesOf, setEstimatesOf] = useState<Employee | null>(null)

  const load = () => employeesApi.list().then(setItems).catch((reason) => setError(errorMessage(reason))).finally(() => setLoading(false))
  useEffect(() => { void load() }, [])
  // Сообщение об успехе исчезает само — не закрывает кнопки в шапке. Ошибка остаётся до закрытия.
  useEffect(() => {
    if (!notice) return
    const timer = window.setTimeout(() => setNotice(''), 4000)
    return () => window.clearTimeout(timer)
  }, [notice])

  // Ответ на изменение не считает сметы — число смет остаётся прежним.
  function replace(employee: Employee) {
    setItems((current) => current.map((item) => item.id === employee.id ? { ...employee, estimateCount: item.estimateCount } : item))
  }

  async function update(employee: Employee, change: EmployeeUpdate, done: string) {
    setError(''); setNotice('')
    try {
      replace(await employeesApi.update(employee.id, change))
      setNotice(done)
      return true
    } catch (reason) {
      setError(errorMessage(reason))
      return false
    }
  }

  async function create(event: FormEvent) {
    event.preventDefault()
    setSaving(true); setError(''); setNotice('')
    try {
      const employee = await employeesApi.create({ ...form, name: form.name.trim(), email: form.email.trim() })
      setItems((current) => [...current, employee])
      setForm(emptyForm)
      setAdding(false)
      setNotice(`Сотрудник ${employee.name} добавлен. Передайте ему email и пароль для входа.`)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setSaving(false)
    }
  }

  async function savePassword(event: FormEvent, employee: Employee) {
    event.preventDefault()
    if (await update(employee, { password: newPassword }, `Пароль для ${employee.name || employee.email} изменён. Сотрудник войдёт заново с новым паролем.`)) {
      setPasswordFor(null)
      setNewPassword('')
    }
  }

  async function saveDetails(event: FormEvent, employee: Employee) {
    event.preventDefault()
    if (!editing) return
    if (await update(employee, { name: editing.name.trim(), email: editing.email.trim() }, `Данные сотрудника ${editing.name.trim() || editing.email.trim()} сохранены.`)) setEditing(null)
  }

  function toggleBlocked(employee: Employee) {
    const who = employee.name || employee.email
    if (!employee.disabled && !window.confirm(`Заблокировать ${who}? Сотрудник сразу выйдет из системы и не сможет войти.`)) return
    void update(employee, { disabled: !employee.disabled }, employee.disabled ? `${who} разблокирован.` : `${who} заблокирован.`)
  }

  async function remove(employee: Employee) {
    const who = employee.name || employee.email
    const estimates = employee.estimateCount > 0 ? ` Его ${employee.estimateCount} ${estimatesWord(employee.estimateCount)} перейдут к вам.` : ''
    if (!window.confirm(`Удалить сотрудника ${who}? Он больше не сможет войти.${estimates} Добавленные им записи справочников останутся, автором станете вы. Отменить удаление нельзя.`)) return
    setError(''); setNotice('')
    try {
      const moved = await employeesApi.remove(employee.id)
      setNotice(`${who} удалён.${moved > 0 ? ` ${moved} ${estimatesWord(moved)} теперь в ваших «Сметах».` : ''}`)
      await load()
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="employees" onLogout={onLogout} />
    <section className="directory-page">
      <div className="dir-head">
        <div>
          <h1>Сотрудники{!loading && <span className="dir-count">{items.length}</span>}</h1>
          <p>Администратор — полный доступ; в «Сметах» видит свои сметы, сметы сотрудников — здесь. Инвойсер и переводчик — калькуляторы.</p>
        </div>
        {!adding && <button type="button" className="dir-add" onClick={() => { setAdding(true); setNotice('') }}>+ Добавить сотрудника</button>}
      </div>
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      {notice && <p className="banner ok" role="status">{notice}<button type="button" onClick={() => setNotice('')}>×</button></p>}
      {adding && <form className="emp-card" onSubmit={(event) => void create(event)}>
        <label>Имя<input value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required maxLength={120} autoFocus /></label>
        <label>Email<input type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} required autoComplete="off" /></label>
        <label>Пароль<input value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} required minLength={8} autoComplete="new-password" placeholder="не короче 8 символов" /></label>
        <label>Роль<select value={form.role} onChange={(event) => setForm({ ...form, role: event.target.value as Role })}>{roles.map((role) => <option key={role} value={role}>{roleLabels[role]}</option>)}</select></label>
        <div className="emp-card-actions">
          <button type="button" className="emp-secondary" onClick={() => { setAdding(false); setForm(emptyForm) }}>Отмена</button>
          <button type="submit" disabled={saving}>{saving ? 'Сохраняем…' : 'Добавить'}</button>
        </div>
      </form>}
      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="dir-table emp-table">
          <thead><tr><th>Сотрудник</th><th>Роль</th><th className="c-status">Статус</th><th className="emp-count-col">Сметы</th><th>Действия</th></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={5} className="dir-empty">{loading ? 'Загрузка…' : 'Сотрудников пока нет.'}</td></tr>
              : items.map((employee) => {
                const self = employee.id === user.id
                const edit = editing?.id === employee.id ? editing : null
                return <tr key={employee.id} className={employee.disabled ? 'archived' : undefined}>
                  <td className="c-name">
                    {edit
                      ? <form id={`emp-edit-${employee.id}`} className="emp-edit" onSubmit={(event) => void saveDetails(event, employee)}>
                        <input value={edit.name} onChange={(event) => setEditing({ ...edit, name: event.target.value })} required maxLength={120} autoFocus placeholder="Имя" aria-label="Имя" />
                        <input type="email" value={edit.email} onChange={(event) => setEditing({ ...edit, email: event.target.value })} required placeholder="Email" aria-label="Email" autoComplete="off" />
                      </form>
                      : <><strong>{employee.name || '—'}{self && <span className="emp-self"> (вы)</span>}</strong><small>{employee.email}</small></>}
                  </td>
                  <td>
                    <select className="emp-role" value={employee.role} disabled={self} title={self ? 'Свою роль администратор изменить не может' : undefined} aria-label={`Роль ${employee.email}`}
                      onChange={(event) => void update(employee, { role: event.target.value as Role }, `${employee.name || employee.email}: роль «${roleLabels[event.target.value as Role]}».`)}>
                      {roles.map((role) => <option key={role} value={role}>{roleLabels[role]}</option>)}
                    </select>
                  </td>
                  <td className="c-status"><span className={`directory-status ${employee.disabled ? 'archived' : 'operator_verified'}`}>{employee.disabled ? 'Заблокирован' : 'Активен'}</span></td>
                  <td className="emp-count-col">
                    <button type="button" className="emp-count" onClick={() => setEstimatesOf(employee)} disabled={employee.estimateCount === 0} title={employee.estimateCount ? 'Открыть сметы сотрудника' : 'Смет нет'}>{employee.estimateCount}</button>
                  </td>
                  <td>
                    {edit
                      // Свой key у каждого набора кнопок: иначе React превратит нажатую «Изменить» в
                      // «Сохранить» (type="submit") прямо во время клика, и браузер сразу отправит форму.
                      ? <div className="emp-actions" key="editing">
                        <button type="submit" form={`emp-edit-${employee.id}`}>Сохранить</button>
                        <button type="button" className="emp-secondary" onClick={() => setEditing(null)}>Отмена</button>
                      </div>
                      : passwordFor === employee.id
                      ? <form className="emp-password" onSubmit={(event) => void savePassword(event, employee)}>
                        <input value={newPassword} onChange={(event) => setNewPassword(event.target.value)} minLength={8} required autoFocus autoComplete="new-password" placeholder="новый пароль" aria-label="Новый пароль" />
                        <button type="submit">Сохранить</button>
                        <button type="button" className="emp-secondary" onClick={() => { setPasswordFor(null); setNewPassword('') }}>Отмена</button>
                      </form>
                      : <div className="emp-actions" key="idle">
                        <button type="button" className="emp-secondary" onClick={(event) => { event.preventDefault(); setEditing({ id: employee.id, name: employee.name, email: employee.email }); setPasswordFor(null) }}>Изменить</button>
                        <button type="button" className="emp-secondary" onClick={() => { setPasswordFor(employee.id); setNewPassword(''); setEditing(null) }}>Сменить пароль</button>
                        {!self && <button type="button" className={employee.disabled ? 'emp-secondary' : 'emp-secondary danger'} onClick={() => toggleBlocked(employee)}>{employee.disabled ? 'Разблокировать' : 'Заблокировать'}</button>}
                        {!self && <button type="button" className="emp-secondary danger" onClick={() => void remove(employee)}>Удалить</button>}
                      </div>}
                  </td>
                </tr>
              })}
          </tbody>
        </table>
      </div>
      <p className="dir-foot">Сотрудники входят по email и паролю, которые вы им задали. Смена пароля, блокировка и удаление сразу завершают их сеансы; сметы удалённого сотрудника переходят к вам.</p>
    </section>
    {estimatesOf && <EmployeeEstimatesPanel employee={estimatesOf} onClose={() => setEstimatesOf(null)} />}
  </main>
}
