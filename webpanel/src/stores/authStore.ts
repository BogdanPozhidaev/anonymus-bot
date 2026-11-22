import { create } from 'zustand';
import type { OperatorRole } from '../types/api';

interface AuthState {
  token: string | null;
  operatorId: number | null;
  role: OperatorRole | null;
  isAuthenticated: boolean;
  passwordChangeRequired: boolean;
  setSession: (token: string, operatorId: number, role: OperatorRole, passwordChangeRequired?: boolean) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  token: null,
  operatorId: null,
  role: null,
  isAuthenticated: false,
  passwordChangeRequired: false,
  setSession: (token, operatorId, role, passwordChangeRequired = false) =>
    set({ token, operatorId, role, passwordChangeRequired, isAuthenticated: true }),
  logout: () =>
    set({ token: null, operatorId: null, role: null, passwordChangeRequired: false, isAuthenticated: false }),
}));
