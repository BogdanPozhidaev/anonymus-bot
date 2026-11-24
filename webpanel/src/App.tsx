import { useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { usePanelLocale } from './i18n/panelLocale';

import { LoginPage } from './pages/LoginPage';
import { PasswordPage } from './pages/PasswordPage';
import { RecoveryCodesPage } from './pages/RecoveryCodesPage';
import { TelegramLinkPage } from './pages/TelegramLinkPage';
import { SessionsPage } from './pages/SessionsPage';
import { DashboardPage } from './pages/DashboardPage';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { OperatorsPage } from './pages/OperatorsPage';
import { IncomingRequestsPage } from './pages/IncomingRequestsPage';
import { AuditLogPage } from './pages/AuditLogPage';
import { AppShell } from './components/layout/AppShell';
import { ProtectedRoute } from './components/layout/ProtectedRoute';

function App() {
  const { language, productName } = usePanelLocale();
  useEffect(() => {
    document.documentElement.lang = language;
    document.title = productName;
  }, [language, productName]);

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/account/password" element={<PasswordPage />} />
          <Route path="/account/recovery" element={<RecoveryCodesPage />} />
          <Route path="/account/telegram" element={<TelegramLinkPage />} />
          <Route path="/sessions" element={<SessionsPage />} />
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
          <Route path="/incoming-requests" element={<IncomingRequestsPage />} />
          <Route path="/incoming-requests/:id" element={<IncomingRequestsPage />} />

          <Route element={<ProtectedRoute adminOnly />}>
            <Route path="/audit-log" element={<AuditLogPage />} />
            <Route path="/operators" element={<OperatorsPage />} />
          </Route>
        </Route>
      </Route>

      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  );
}

export default App;
