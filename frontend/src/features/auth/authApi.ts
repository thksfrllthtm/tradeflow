import { request } from '@/shared/api/client'
import { User } from '@/shared/types/user'

export const authApi = {
  me: async () => (await request<{ user: User }>('/api/auth/me')).user,
  login: async (email: string, password: string) =>
    (await request<{ user: User }>('/api/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) })).user,
  register: async (name: string, email: string, password: string) =>
    (await request<{ user: User }>('/api/auth/register', { method: 'POST', body: JSON.stringify({ name, email, password }) })).user,
  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),
}
