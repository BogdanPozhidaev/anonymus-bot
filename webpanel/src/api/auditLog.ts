import { apiClient } from './client';
import type { AuditLogEntry } from '../types/api';

export interface AuditLogFilters {
  actor_id?: number;
  action?: string;
  target_type?: string;
  date_from?: string;
  date_to?: string;
}

export async function listAuditLog(filters: AuditLogFilters = {}): Promise<AuditLogEntry[]> {
  const response = await apiClient.get<{ entries: AuditLogEntry[] }>('/audit-log', {
    params: filters,
  });
  return response.data.entries;
}

export async function downloadAuditLogCsv(filters: AuditLogFilters = {}): Promise<void> {
  const response = await apiClient.get('/audit-log', {
    params: { ...filters, format: 'csv' },
    responseType: 'blob',
  });

  const url = window.URL.createObjectURL(new Blob([response.data]));
  const link = document.createElement('a');
  link.href = url;
  link.download = 'audit_log.csv';
  document.body.appendChild(link);
  link.click();
  link.remove();
  window.URL.revokeObjectURL(url);
}