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

export interface OperatorReply {
  recipientRole: 'client' | 'executor';
  recipientUserId: number;
  text: string;
  requestKey: string;
}

export function createOperatorReplyKey() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export async function sendAsParticipant(sessionId: number, reply: OperatorReply) {
  await apiClient.post(`/sessions/${sessionId}/send-as`, {
    impersonated_role: reply.recipientRole === 'client' ? 'executor' : 'client',
    recipient_role: reply.recipientRole,
    recipient_user_id: reply.recipientUserId,
    request_key: reply.requestKey,
    content_type: 'text',
    content: reply.text,
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
