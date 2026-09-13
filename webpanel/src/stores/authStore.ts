import { create } from 'zustand';
import type { OperatorRole } from '../types/api';

interface AuthState {
  token: string | null;
  operatorId: number | null;
  role: OperatorRole | null;
  isAuthenticated: boolean;
  setSession: (token: string, operatorId: number, role: OperatorRole) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  token: null,
  operatorId: null,
  role: null,
  isAuthenticated: false,
  setSession: (token, operatorId, role) =>
    set({ token, operatorId, role, isAuthenticated: true }),
  logout: () =>
    set({ token: null, operatorId: null, role: null, isAuthenticated: false }),
}));