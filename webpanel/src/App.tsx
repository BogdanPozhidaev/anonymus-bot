import { Routes, Route, Navigate } from 'react-router-dom';

import { LoginPage } from './pages/LoginPage';
import { SessionsPage } from './pages/SessionsPage';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { OperatorsPage } from './pages/OperatorsPage';
import { IncomingRequestsPage } from './pages/IncomingRequestsPage';
import { AuditLogPage } from './pages/AuditLogPage';
import { AppShell } from './components/layout/AppShell';
import { ProtectedRoute } from './components/layout/ProtectedRoute';

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route path="/sessions" element={<SessionsPage />} />
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
          <Route path="/incoming-requests" element={<IncomingRequestsPage />} />
          <Route path="/audit-log" element={<AuditLogPage />} />

          <Route element={<ProtectedRoute adminOnly />}>
            <Route path="/operators" element={<OperatorsPage />} />
          </Route>
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/sessions" replace />} />
    </Routes>
  );
}

export default App;