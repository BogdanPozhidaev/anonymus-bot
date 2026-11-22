import { apiClient } from './client';
import type { OperatorListItem, OperatorRole, OperatorStatus, CreateOperatorResponse } from '../types/api';

export async function listOperators(limit: number, offset: number) {
  const response = await apiClient.get<{ operators: OperatorListItem[]; has_more: boolean }>('/operators', {
    params: { limit, offset },
  });
  return response.data;
}

export async function createOperator(
  name: string,
  login: string,
  role: OperatorRole,
  email?: string
): Promise<CreateOperatorResponse> {
  const response = await apiClient.post<CreateOperatorResponse>('/operators', {
    name,
    login,
    role,
    email,
  });
  return response.data;
}

export async function updateOperator(
  id: number,
  changes: { name?: string; role?: OperatorRole; status?: OperatorStatus }
): Promise<void> {
  await apiClient.patch(`/operators/${id}`, changes);
}

export async function resetOperatorPassword(id: number): Promise<CreateOperatorResponse> {
  const response = await apiClient.post<CreateOperatorResponse>(`/operators/${id}/password/reset`);
  return response.data;
}
