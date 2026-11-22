import { usePanelLocale } from '../i18n/panelLocale';
import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Table, Title, Button, Text, Loader, Badge, Group, Alert } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { listIncomingRequests, markIncomingRequestProcessed } from '../api/incomingRequests';
import { createSession } from '../api/sessions';
import { ListPager } from '../components/ListPager';

const pageSize = 50;

export function IncomingRequestsPage() {
  const { t, locale } = usePanelLocale();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<'new' | 'processed'>('new');

  const [offset, setOffset] = useState(0);
  const { data, isLoading, isError } = useQuery({
    queryKey: ['incoming-requests', status, offset],
    queryFn: () => listIncomingRequests(status, pageSize, offset),
    refetchInterval: 10000,
  });
  const requests = data?.requests;

  const createSessionMutation = useMutation({
    mutationFn: async (requestId: number) => {
      const request = requests?.find((r) => r.id === requestId);
      const title = request?.first_name
        ? t('requestFrom', { name: request.first_name })
        : t('requestNumber', { id: requestId });

      const session = await createSession(title);
      await markIncomingRequestProcessed(requestId);
      return session;
    },
    onSuccess: (session) => {
      queryClient.invalidateQueries({ queryKey: ['incoming-requests'] });
      notifications.show({ message: t('sessionCreated'), color: 'green' });
      navigate(`/sessions/${session.id}`);
    },
    onError: () => {
      notifications.show({ message: t('sessionCreateFailed'), color: 'red' });
    },
  });

  return (
    <>
      <Title order={2} mb="md">
        {t('requests')}
      </Title>

      <Group mb="md">
        <Button variant={status === 'new' ? 'filled' : 'default'} aria-pressed={status === 'new'} onClick={() => { setStatus('new'); setOffset(0); }}>
          {t('newItems')}
        </Button>
        <Button variant={status === 'processed' ? 'filled' : 'default'} aria-pressed={status === 'processed'} onClick={() => { setStatus('processed'); setOffset(0); }}>
          {t('processedItems')}
        </Button>
      </Group>
      {isLoading && <Loader />}
      {isError && <Alert color="red">{t('requestsLoadFailed')}</Alert>}
      {requests && <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('name')}</Table.Th>
            <Table.Th>{t('telegramUsername')}</Table.Th>
            <Table.Th>{t('firstMessage')}</Table.Th>
            <Table.Th>{t('language')}</Table.Th>
            <Table.Th>{t('date')}</Table.Th>
            <Table.Th></Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {requests?.map((req) => (
            <Table.Tr key={req.id}>
              <Table.Td>{req.first_name || '—'}</Table.Td>
              <Table.Td>{req.username ? `@${req.username}` : '—'}</Table.Td>
              <Table.Td maw={300}>
                <Text size="sm" lineClamp={2}>
                  {req.first_message_text || '—'}
                </Text>
              </Table.Td>
              <Table.Td>
                <Badge variant="light">{req.language_code || '—'}</Badge>
              </Table.Td>
              <Table.Td>{new Date(req.created_at).toLocaleString(locale)}</Table.Td>
              <Table.Td>
                {status === 'new' ? <Button
                  size="xs"
                  onClick={() => createSessionMutation.mutate(req.id)}
                  loading={createSessionMutation.isPending}
                >
                  {t('createSession')}
                </Button> : <Badge variant="light">{t('processed')}</Badge>}
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>}
      <ListPager offset={offset} size={pageSize} hasMore={data?.has_more ?? false} loading={isLoading} onChange={setOffset} />

      {requests?.length === 0 && (
        <Text c="dimmed" ta="center" mt="xl">
          {status === 'new' ? t('noNewRequests') : t('noProcessedRequests')}
        </Text>
      )}
    </>
  );
}
