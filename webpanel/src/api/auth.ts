import { apiClient } from './client';
import type { LoginResponse, VerifyTotpResponse } from '../types/api';

export async function login(login: string, password: string): Promise<LoginResponse> {
  const response = await apiClient.post<LoginResponse>('/auth/login', { login, password });
  return response.data;
}

export async function verifyTotp(pendingToken: string, code: string): Promise<VerifyTotpResponse> {
  const response = await apiClient.post<VerifyTotpResponse>('/auth/totp/verify', {
    pending_token: pendingToken,
    code,
  });
  return response.data;
}

export async function logout(): Promise<void> {
  await apiClient.post('/auth/logout');
}