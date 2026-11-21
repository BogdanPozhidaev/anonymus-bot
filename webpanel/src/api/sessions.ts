import { apiClient } from './client';
import type { SessionListItem, SessionStatus } from '../types/api';

export async function listSessions(params: { q?: string; status?: SessionStatus; limit: number; offset: number }) {
  const response = await apiClient.get<{ sessions: SessionListItem[]; has_more: boolean }>('/sessions', { params });
  return response.data;
}

export async function getSession(id: number): Promise<SessionListItem> {
  const response = await apiClient.get<SessionListItem>(`/sessions/${id}`);
  return response.data;
}

export async function createSession(title: string, language = 'ru'): Promise<SessionListItem> {
  const response = await apiClient.post<SessionListItem>('/sessions', { title, language });
  return response.data;
}

export async function updateSessionStatus(id: number, status: SessionStatus): Promise<void> {
  await apiClient.patch(`/sessions/${id}/status`, { status });
}

export async function assignSessionOwner(id: number, operatorId: number): Promise<void> {
  await apiClient.patch(`/sessions/${id}/owner`, { operator_id: operatorId });
}
export async function issueSessionInvite(id: number, role: 'client' | 'executor') {
  const response = await apiClient.post<{ token: string; expires_at: string }>(`/sessions/${id}/invites`, { role });
  return response.data;
}

export async function revokeSessionInvite(id: number, role: 'client' | 'executor'): Promise<void> {
  await apiClient.delete(`/sessions/${id}/invites/${role}`);
}
