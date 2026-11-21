import { AppShell as MantineAppShell, NavLink, Group, Text, Button } from '@mantine/core';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import {
  IconMessageCircle,
  IconUsers,
  IconInbox,
  IconHistory,
  IconLogout,
} from '@tabler/icons-react';

import { useAuthStore } from '../../stores/authStore';
import { logout as logoutRequest } from '../../api/auth';
import { panelConfig } from '../../panelConfig';

export function AppShell() {
  const navigate = useNavigate();
  const location = useLocation();
  const { role, logout } = useAuthStore();

  const handleLogout = async () => {
    try {
      await logoutRequest();
    } catch {
      // Даже если запрос на backend не удался, разлогиниваем локально —
      // токен уже мог истечь, нет смысла блокировать пользователя.
    }
    logout();
    navigate('/login');
  };

  const navItems = [
    { path: '/sessions', label: 'Сессии', icon: IconMessageCircle },
    { path: '/incoming-requests', label: 'Входящие обращения', icon: IconInbox },
  ];

  if (role === 'admin') {
    navItems.push({ path: '/audit-log', label: 'Аудит', icon: IconHistory });
    navItems.push({ path: '/operators', label: 'Операторы', icon: IconUsers });
  }

  return (
    <MantineAppShell navbar={{ width: 260, breakpoint: 'sm' }} padding="md">
      <MantineAppShell.Navbar p="md">
        <Text fw={700} size="lg" mb="md">
          {panelConfig().productName}
        </Text>
        {navItems.map((item) => (
          <NavLink
            key={item.path}
            label={item.label}
            leftSection={<item.icon size={18} />}
            active={location.pathname.startsWith(item.path)}
            onClick={() => navigate(item.path)}
          />
        ))}
        <Group mt="auto" pt="md">
          <Button
            variant="subtle"
            color="red"
            leftSection={<IconLogout size={18} />}
            onClick={handleLogout}
            fullWidth
          >
            Выйти
          </Button>
        </Group>
      </MantineAppShell.Navbar>

      <MantineAppShell.Main>
        <Outlet />
      </MantineAppShell.Main>
    </MantineAppShell>
  );
}
