import { apiClient } from './client';
import type { OperatorListItem, OperatorRole, OperatorStatus, CreateOperatorResponse } from '../types/api';

export async function listOperators(): Promise<OperatorListItem[]> {
  const response = await apiClient.get<{ operators: OperatorListItem[] }>('/operators');
  return response.data.operators;
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