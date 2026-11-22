import { apiClient } from './client';
import { saveDownload } from './download';
import { useAuthStore } from '../stores/authStore';
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

export type TranscriptFormat = 'csv' | 'pdf';

export async function downloadTranscript(sessionId: number, format: TranscriptFormat) {
  const token = useAuthStore.getState().token;
  const { data } = await apiClient.get<Blob>(`/sessions/${sessionId}/transcript`, {
    params: { format },
    responseType: 'blob',
  });
  if (token && useAuthStore.getState().token === token) {
    saveDownload(data, `session-${sessionId}.${format}`);
  }
}
