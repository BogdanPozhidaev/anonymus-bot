import { Navigate, Outlet, useLocation } from 'react-router-dom';
import { useAuthStore } from '../../stores/authStore';

interface ProtectedRouteProps {
  adminOnly?: boolean;
}

export function ProtectedRoute({ adminOnly }: ProtectedRouteProps) {
  const { isAuthenticated, role, passwordChangeRequired } = useAuthStore();
  const { pathname } = useLocation();

  if (!isAuthenticated) {
    const returnTo = /^\/incoming-requests\/\d+$/.test(pathname) ? pathname : undefined;
    return <Navigate to="/login" replace state={{ returnTo }} />;
  }

  if (passwordChangeRequired && pathname !== '/account/password') {
    return <Navigate to="/account/password" replace />;
  }

  if (adminOnly && role !== 'admin') {
    return <Navigate to="/sessions" replace />;
  }

  return <Outlet />;
}
