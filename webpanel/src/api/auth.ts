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

export async function changePassword(current: string, next: string): Promise<void> {
  await apiClient.post('/auth/password', { current_password: current, new_password: next });
}

export async function recoverTotp(pendingToken: string, recoveryCode: string): Promise<LoginResponse> {
  const response = await apiClient.post<LoginResponse>('/auth/totp/recover', {
    pending_token: pendingToken,
    recovery_code: recoveryCode,
  });
  return response.data;
}

export async function issueRecoveryCodes(password: string, code: string): Promise<string[]> {
  const response = await apiClient.post<{ codes: string[] }>('/auth/totp/recovery-codes', { password, code });
  return response.data.codes;
}
