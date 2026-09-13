export type OperatorRole = 'admin' | 'operator';
export type OperatorStatus = 'invited' | 'active' | 'suspended' | 'disabled';
export type SessionStatus = 'active' | 'paused' | 'pending_close' | 'closed';

export interface LoginResponse {
  status: 'totp_required' | 'totp_setup_required' | 'ok';
  pending_token?: string;
  session_token?: string;
  totp_secret?: string;
  totp_qr_url?: string;
}

export interface VerifyTotpResponse {
  session_token: string;
  operator_id: number;
  role: OperatorRole;
}

export interface SessionListItem {
  id: number;
  title: string;
  status: SessionStatus;
  owner_operator_id: number | null;
  client_user_id: number | null;
  executor_user_id: number | null;
  created_at: string;
}

export interface OperatorListItem {
  id: number;
  name: string;
  login: string;
  email: string | null;
  role: OperatorRole;
  status: OperatorStatus;
  created_at: string;
  last_login_at: string | null;
}

export interface CreateOperatorResponse {
  id: number;
  login: string;
  temporary_password: string;
}

export interface AuditLogEntry {
  id: number;
  timestamp: string;
  actor_id: number | null;
  actor_role: string | null;
  action: string;
  target_type: string | null;
  target_id: number | null;
  payload: Record<string, unknown> | null;
  ip_address: string | null;
  user_agent: string | null;
}

export interface Message {
  id: number;
  session_id: number;
  sender_user_id: number | null;
  sender_role: 'client' | 'executor' | 'operator';
  content_type: 'text' | 'photo' | 'voice' | 'system';
  content: string | null;
  file_id: string | null;
  sent_by_operator: boolean;
  impersonated_role: string | null;
  delivered: boolean;
  created_at: string;
}

export interface IncomingRequest {
  id: number;
  telegram_id: number;
  username: string | null;
  first_name: string | null;
  first_message_text: string | null;
  language_code: string | null;
  status: 'new' | 'processed';
  created_at: string;
}