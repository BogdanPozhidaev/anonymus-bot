import { apiClient } from './client';
import type { IncomingRequest } from '../types/api';

export async function listIncomingRequests(status?: string): Promise<IncomingRequest[]> {
  const response = await apiClient.get<{ requests: IncomingRequest[] }>('/incoming-requests', {
    params: status ? { status } : undefined,
  });
  return response.data.requests;
}

export async function markIncomingRequestProcessed(id: number): Promise<void> {
  await apiClient.patch(`/incoming-requests/${id}/status`, { status: 'processed' });
}