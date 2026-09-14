import { apiClient } from './client';
import type { Message } from '../types/api';

export async function listMessages(sessionId: number): Promise<Message[]> {
  const response = await apiClient.get<{ messages: Message[] }>(`/sessions/${sessionId}/messages`);
  return response.data.messages;
}