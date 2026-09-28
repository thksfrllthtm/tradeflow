export type Role = 'admin' | 'invoicer' | 'translator'

export type User = { id: number; name: string; email: string; role: Role }

export const roleLabels: Record<Role, string> = {
  admin: 'Администратор',
  invoicer: 'Инвойсер',
  translator: 'Переводчик',
}

// Администратор управляет сотрудниками и справочниками и видит сметы всех сотрудников.
// Остальные работают в калькуляторах, а справочники только просматривают.
export const isAdmin = (user: User) => user.role === 'admin'

// Исключение: инвойсер добавляет недостающий код ТН ВЭД сам, но «Проверено» ставит только администратор.
export const canAddTNVED = (user: User) => user.role === 'admin' || user.role === 'invoicer'
