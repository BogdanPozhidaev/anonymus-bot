import { apiClient } from './client';
import type { Message } from '../types/api';

export async function listMessages(sessionId: number, limit: number, offset: number) {
  const response = await apiClient.get<{ messages: Message[]; has_more: boolean }>(`/sessions/${sessionId}/messages`, {
    params: { limit, offset },
  });
  return response.data;
}

export async function sendAsParticipant(sessionId: number, role: 'client' | 'executor', text: string) {
  await apiClient.post(`/sessions/${sessionId}/send-as`, {
    impersonated_role: role,
    content_type: 'text',
    content: text,
  });
}
