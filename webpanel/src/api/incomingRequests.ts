import { apiClient } from './client';
import type { IncomingRequest } from '../types/api';

export async function listIncomingRequests(status: string, limit: number, offset: number) {
  const response = await apiClient.get<{ requests: IncomingRequest[]; has_more: boolean }>('/incoming-requests', {
    params: { status, limit, offset },
  });
  return response.data;
}

export async function markIncomingRequestProcessed(id: number): Promise<void> {
  await apiClient.patch(`/incoming-requests/${id}/status`, { status: 'processed' });
}
