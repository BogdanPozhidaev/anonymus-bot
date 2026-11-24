import { apiClient } from './client';
import type { IncomingRequest } from '../types/api';

export async function listIncomingRequests(status: string, limit: number, offset: number, allAssigned = false) {
  const response = await apiClient.get<{ requests: IncomingRequest[]; has_more: boolean }>('/incoming-requests', {
    params: { status, limit, offset, scope: allAssigned ? 'all' : undefined },
  });
  return response.data;
}

export async function transferIncomingRequest(id: number, expectedOwnerId: number, operatorId: number) {
  await apiClient.post(`/incoming-requests/${id}/transfer`, { expected_owner_id: expectedOwnerId, operator_id: operatorId });
}

export async function getIncomingRequest(id: string) {
  const response = await apiClient.get<IncomingRequest>(`/incoming-requests/${encodeURIComponent(id)}`);
  return response.data;
}

export async function claimIncomingRequest(id: number) {
  await apiClient.post(`/incoming-requests/${id}/claim`);
}

export async function openIncomingRequestSession(id: number, title: string) {
  const response = await apiClient.post<{ id: number }>(`/incoming-requests/${id}/session`, { title });
  return response.data;
}
