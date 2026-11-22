import { Alert, Badge, Button, Group, Loader, Paper, SimpleGrid, Stack, Text, Title } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { listSessions } from '../api/sessions';
import { listIncomingRequests } from '../api/incomingRequests';
import { sessionStatusMessages, usePanelLocale } from '../i18n/panelLocale';

const previewSize = 5;

export function DashboardPage() {
  const { t, locale } = usePanelLocale();
  const sessions = useQuery({
    queryKey: ['sessions', 'dashboard'],
    queryFn: () => listSessions({ limit: previewSize, offset: 0 }),
  });
  const requests = useQuery({
    queryKey: ['incoming-requests', 'dashboard'],
    queryFn: () => listIncomingRequests('new', previewSize, 0),
  });

  return (
    <Stack>
      <Title order={2}>{t('dashboard')}</Title>
      <SimpleGrid cols={{ base: 1, lg: 2 }}>
        <Paper withBorder p="md">
          <Stack>
            <Group justify="space-between">
              <Title order={3}>{t('recentSessions')}</Title>
              <Button component={Link} to="/sessions" size="compact-sm" variant="subtle">{t('viewSessions')}</Button>
            </Group>
            {sessions.isPending && <Loader size="sm" />}
            {sessions.isError && <Alert color="red">{t('sessionsLoadFailed')}</Alert>}
            {sessions.data && (
              sessions.data.sessions.length ? sessions.data.sessions.map(session => (
                <Group key={session.id} justify="space-between">
                  <Text component={Link} to={`/sessions/${session.id}`} style={{ overflowWrap: 'anywhere' }}>{session.title}</Text>
                  <Badge variant="light">{t(sessionStatusMessages[session.status])}</Badge>
                </Group>
              )) : <Text c="dimmed">{t('emptySessions')}</Text>
            )}
          </Stack>
        </Paper>
        <Paper withBorder p="md">
          <Stack>
            <Group justify="space-between">
              <Title order={3}>{t('dashboardRequests')}</Title>
              <Button component={Link} to="/incoming-requests" size="compact-sm" variant="subtle">{t('viewRequests')}</Button>
            </Group>
            {requests.isPending && <Loader size="sm" />}
            {requests.isError && <Alert color="red">{t('requestsLoadFailed')}</Alert>}
            {requests.data && (
              requests.data.requests.length ? requests.data.requests.map(request => (
                <Group key={request.id} justify="space-between">
                  <Text style={{ overflowWrap: 'anywhere' }}>
                    {request.first_name ? t('requestFrom', { name: request.first_name }) : t('requestNumber', { id: request.id })}
                  </Text>
                  <Text size="sm" c="dimmed">{new Date(request.created_at).toLocaleString(locale)}</Text>
                </Group>
              )) : <Text c="dimmed">{t('noNewRequests')}</Text>
            )}
          </Stack>
        </Paper>
      </SimpleGrid>
    </Stack>
  );
}
