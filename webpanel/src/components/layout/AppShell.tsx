import { usePanelLocale } from '../../i18n/panelLocale';
import { AppShell as MantineAppShell, NavLink, Group, Text, Button, Burger } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { useQueryClient } from '@tanstack/react-query';
import { Link, Outlet, useNavigate, useLocation } from 'react-router-dom';
import {
  IconMessageCircle,
  IconUsers,
  IconInbox,
  IconHistory,
  IconLogout,
  IconKey,
  IconLock,
  IconBrandTelegram,
  IconDashboard,
} from '@tabler/icons-react';

import { useAuthStore } from '../../stores/authStore';
import { logout as logoutRequest } from '../../api/auth';
import { LanguageSelector } from '../LanguageSelector';

export function AppShell() {
  const { t, productName } = usePanelLocale();
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const { role, logout, passwordChangeRequired } = useAuthStore();
  const [mobileMenuOpened, { toggle, close }] = useDisclosure(false);

  const handleLogout = async () => {
    try {
      await logoutRequest();
    } catch {
      // Discard the local session even if the API sign-out fails.
    }
    queryClient.clear();
    logout();
    navigate('/login');
  };

  const navItems = [
    { path: '/dashboard', label: t('dashboard'), icon: IconDashboard },
    { path: '/sessions', label: t('sessions'), icon: IconMessageCircle },
    { path: '/incoming-requests', label: t('requests'), icon: IconInbox },
    { path: '/account/password', label: t('passwordChange'), icon: IconKey },
    { path: '/account/recovery', label: t('recoveryCodes'), icon: IconLock },
    { path: '/account/telegram', label: t('telegramAccount'), icon: IconBrandTelegram },
  ];

  if (role === 'admin') {
    navItems.push({ path: '/audit-log', label: t('audit'), icon: IconHistory });
    navItems.push({ path: '/operators', label: t('operators'), icon: IconUsers });
  }

  return (
    <MantineAppShell header={{ height: { base: 48, sm: 0 } }}
      navbar={{ width: 260, breakpoint: 'sm', collapsed: { mobile: !mobileMenuOpened } }} padding="md">
      <MantineAppShell.Header hiddenFrom="sm">
        <Group h="100%" px="md">
          <Burger opened={mobileMenuOpened} onClick={toggle} size="sm"
            aria-label={t(mobileMenuOpened ? 'closeNavigation' : 'openNavigation')} aria-expanded={mobileMenuOpened} />
          <Text fw={700}>{productName}</Text>
        </Group>
      </MantineAppShell.Header>
      <MantineAppShell.Navbar p="md">
        <Text fw={700} size="lg" mb="md">
          {productName}
        </Text>
        {!passwordChangeRequired && navItems.map((item) => (
          <NavLink
            key={item.path}
            component={Link}
            to={item.path}
            label={item.label}
            leftSection={<item.icon size={18} />}
            active={location.pathname.startsWith(item.path)}
            onClick={close}
          />
        ))}
        <Group mt="auto" pt="md">
          <LanguageSelector />
          <Button
            variant="subtle"
            color="red"
            leftSection={<IconLogout size={18} />}
            onClick={handleLogout}
            fullWidth
          >
            {t('signOut')}
          </Button>
        </Group>
      </MantineAppShell.Navbar>

      <MantineAppShell.Main>
        <Outlet />
      </MantineAppShell.Main>
    </MantineAppShell>
  );
}
