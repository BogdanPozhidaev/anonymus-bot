import { usePanelLocale } from '../i18n/panelLocale';
import { useRef, useState } from 'react';
import { isAxiosError } from 'axios';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { Table, Title, Button, Text, Loader, Badge, Group, Alert, Select, Stack } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { claimIncomingRequest, getIncomingRequest, listIncomingRequests, openIncomingRequestSession, transferIncomingRequest } from '../api/incomingRequests';
import { listOperators } from '../api/operators';
import { ListPager } from '../components/ListPager';
import { useAuthStore } from '../stores/authStore';

const pageSize = 50;

export function IncomingRequestsPage() {
  const { t, locale } = usePanelLocale();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { id } = useParams();
  const heading = useRef<HTMLHeadingElement>(null);
  const [notice, setNotice] = useState<'requestClaimed' | 'requestClaimConflict' | 'claimRequestFailed' | 'requestSessionUncertain' | null>(null);
  const isAdmin = useAuthStore(state => state.role === 'admin');
  const [status, setStatus] = useState<'new' | 'in_progress' | 'processed' | 'all_assigned'>('new');

  const [offset, setOffset] = useState(0);
  const { data, isLoading, isError } = useQuery({
    queryKey: ['incoming-requests', status, offset],
    queryFn: () => listIncomingRequests(status === 'all_assigned' ? 'in_progress' : status, pageSize, offset, status === 'all_assigned'),
    enabled: !id,
    refetchInterval: 10000,
  });
  const detail = useQuery({
    queryKey: ['incoming-requests', 'detail', id],
    queryFn: () => getIncomingRequest(id ?? ''),
    enabled: Boolean(id),
  });
  const unavailable = id && isAxiosError(detail.error) && [403, 404].includes(detail.error.response?.status ?? 0);
  const selectedRequest = unavailable ? undefined : detail.data;
  const requests = id ? selectedRequest && [selectedRequest] : data?.requests;
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['incoming-requests'] });

  const claimMutation = useMutation({
    mutationFn: claimIncomingRequest,
    onSuccess: () => {
      setStatus('in_progress');
      setOffset(0);
      setNotice('requestClaimed');
      void refresh();
      heading.current?.focus();
    },
    onError: (error) => {
      setNotice(isAxiosError(error) && error.response?.status === 409 ? 'requestClaimConflict' : 'claimRequestFailed');
      void refresh();
    },
  });

  const createSessionMutation = useMutation({
    mutationFn: (request: NonNullable<typeof requests>[number]) => {
      const title = request.first_name
        ? t('requestFrom', { name: request.first_name })
        : t('requestNumber', { id: request.id });
      return openIncomingRequestSession(request.id, title.slice(0, 200));
    },
    onSuccess: (session) => {
      void refresh();
      notifications.show({ message: t('sessionCreated'), color: 'green' });
      navigate(`/sessions/${session.id}`);
    },
    onError: () => {
      setNotice('requestSessionUncertain');
      void refresh();
    },
  });

  return (
    <>
      <Title order={2} mb="md" ref={heading} tabIndex={-1}>
        {id ? t('requestNumber', { id }) : t('requests')}
      </Title>

      {id ? <Button component={Link} to="/incoming-requests" mb="md" variant="light">{t('requests')}</Button> : <Group mb="md">
        <Button variant={status === 'new' ? 'filled' : 'default'} aria-pressed={status === 'new'} onClick={() => { setStatus('new'); setOffset(0); }}>
          {t('newItems')}
        </Button>
        <Button variant={status === 'in_progress' ? 'filled' : 'default'} aria-pressed={status === 'in_progress'} onClick={() => { setStatus('in_progress'); setOffset(0); }}>
          {t('myRequests')}
        </Button>
        {isAdmin && <Button variant={status === 'all_assigned' ? 'filled' : 'default'} aria-pressed={status === 'all_assigned'}
          onClick={() => { setStatus('all_assigned'); setOffset(0); }}>
          {t('allAssignedRequests')}
        </Button>}
        <Button variant={status === 'processed' ? 'filled' : 'default'} aria-pressed={status === 'processed'} onClick={() => { setStatus('processed'); setOffset(0); }}>
          {t('processedItems')}
        </Button>
      </Group>}
      <Text component="div" role="status" aria-atomic="true" mb={notice ? 'sm' : 0}>{notice && t(notice)}</Text>
      {(id ? detail.isLoading : isLoading) && <Loader aria-label={t('requestsLoading')} />}
      {(id ? detail.isError : isError) && <Alert color="red">
        <Text>{t(unavailable ? 'requestUnavailable' : 'requestsLoadFailed')}</Text>
        <Button variant="light" size="xs" mt="xs" onClick={() => void refresh()}>{t('refreshRequests')}</Button>
      </Alert>}
      {requests && <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('name')}</Table.Th>
            <Table.Th>{t('telegramUsername')}</Table.Th>
            <Table.Th>{t('firstMessage')}</Table.Th>
            <Table.Th>{t('language')}</Table.Th>
            <Table.Th>{t('date')}</Table.Th>
            <Table.Th>{t('status')}</Table.Th>
            <Table.Th></Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {requests?.map((req) => (
            <Table.Tr key={req.id}>
              <Table.Td>{id ? req.first_name || '—' : <Text component={Link} to={`/incoming-requests/${req.id}`}>
                {req.first_name || t('requestNumber', { id: req.id })}
              </Text>}</Table.Td>
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
              <Table.Td><Badge variant="light">{t(req.status === 'in_progress' ? 'requestInProgress' : req.status === 'processed' ? 'processed' : 'requestNew')}</Badge></Table.Td>
              <Table.Td>
                {req.status === 'new' && <Button
                  size="xs"
                  onClick={() => { setNotice(null); claimMutation.mutate(req.id); }}
                  loading={claimMutation.isPending && claimMutation.variables === req.id}
                  disabled={claimMutation.isPending}
                >
                  {t('claimRequest')}
                </Button>}
                {req.status === 'in_progress' && req.session_id == null && <Button
                  size="xs"
                  onClick={() => { setNotice(null); createSessionMutation.mutate(req); }}
                  loading={createSessionMutation.isPending && createSessionMutation.variables?.id === req.id}
                  disabled={createSessionMutation.isPending}
                >
                  {t('createSession')}
                </Button>}
                {req.session_id != null && <Button size="xs" component={Link} to={`/sessions/${req.session_id}`}>{t('openRequestSession')}</Button>}
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>}
      {isAdmin && id && selectedRequest?.status === 'in_progress' && selectedRequest.owner_operator_id !== null &&
        <RequestTransfer key={`${id}:${selectedRequest.owner_operator_id}`}
          requestId={selectedRequest.id} ownerId={selectedRequest.owner_operator_id} />}
      {!id && <ListPager offset={offset} size={pageSize} hasMore={data?.has_more ?? false} loading={isLoading} onChange={setOffset} />}

      {requests?.length === 0 && (
        <Text c="dimmed" ta="center" mt="xl">
          {t(status === 'new' ? 'noNewRequests' : status === 'in_progress' ? 'noMyRequests' :
            status === 'all_assigned' ? 'noAssignedRequests' : 'noProcessedRequests')}
        </Text>
      )}
    </>
  );
}

function RequestTransfer({ requestId, ownerId }: { requestId: number; ownerId: number }) {
  const { t } = usePanelLocale();
  const queryClient = useQueryClient();
  const [offset, setOffset] = useState(0);
  const [target, setTarget] = useState<string | null>(null);
  const { data, isLoading, isError } = useQuery({
    queryKey: ['operators', offset],
    queryFn: () => listOperators(pageSize, offset),
  });
  const transfer = useMutation({
    mutationFn: (operatorId: number) => transferIncomingRequest(requestId, ownerId, operatorId),
    onSuccess: () => {
      notifications.show({ message: t('requestTransferred'), color: 'green' });
      void queryClient.invalidateQueries({ queryKey: ['incoming-requests'] });
      void queryClient.invalidateQueries({ queryKey: ['session'] });
      void queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
    onError: () => {
      notifications.show({ message: t('requestTransferFailed'), color: 'red' });
      void queryClient.invalidateQueries({ queryKey: ['incoming-requests'] });
    },
  });
  const owner = data?.operators.find(operator => operator.id === ownerId);
  return <Stack gap="xs" mt="md">
    <Text>{t('requestCurrentOwner', { owner: owner?.name ?? t('operatorNumber', { id: ownerId }) })}</Text>
    <form onSubmit={event => {
      event.preventDefault();
      if (target && !transfer.isPending) transfer.mutate(Number(target));
    }}>
      <Group align="end">
        <Select label={t('requestNewOwner')} value={target} onChange={setTarget}
          error={isError ? t('operatorListLoadFailed') : undefined}
          disabled={transfer.isPending || isLoading}
          data={(data?.operators ?? []).map(operator => ({
            value: String(operator.id), label: operator.name,
            disabled: operator.id === ownerId || operator.status !== 'active',
          }))} />
        <Button type="submit" disabled={!target} loading={transfer.isPending}>{t('transferRequest')}</Button>
      </Group>
    </form>
    <ListPager offset={offset} size={pageSize} hasMore={data?.has_more ?? false}
      loading={transfer.isPending || isLoading} onChange={value => { setTarget(null); setOffset(value); }} />
  </Stack>;
}
