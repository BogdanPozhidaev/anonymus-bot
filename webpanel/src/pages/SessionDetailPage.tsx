import { useEffect, useRef, useState } from 'react';
import { sessionStatusMessages, usePanelLocale } from '../i18n/panelLocale';
import { useParams } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import {
  Title,
  Badge,
  Group,
  Paper,
  Stack,
  Text,
  Button,
  CopyButton,
  Tooltip,
  ActionIcon,
  Divider,
  ScrollArea,
  Loader,
  Modal,
  Select,
  Textarea,
} from '@mantine/core';
import { IconCopy, IconCheck } from '@tabler/icons-react';
import { notifications } from '@mantine/notifications';

import { assignSessionOwner, getSession, issueSessionInvite, revokeSessionInvite, updateSessionStatus } from '../api/sessions';
import { createOperatorReplyKey, downloadTranscript, listMessages, sendAsParticipant, type OperatorReply, type TranscriptFormat } from '../api/messages';
import { listOperators } from '../api/operators';
import { panelConfig } from '../panelConfig';
import { useAuthStore } from '../stores/authStore';
import { ListPager } from '../components/ListPager';
import type { SessionStatus } from '../types/api';

const statusColors: Record<SessionStatus, string> = {
  active: 'green',
  paused: 'yellow',
  pending_close: 'orange',
  closed: 'gray',
};

const historyPageSize = 50;

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  return <SessionDetail key={id} sessionId={Number(id)} />;
}

function SessionDetail({ sessionId }: { sessionId: number }) {
  const { t } = usePanelLocale();
  const queryClient = useQueryClient();
  const [confirmClose, setConfirmClose] = useState(false);
  const [offset, setOffset] = useState(0);
  const isAdmin = useAuthStore(state => state.role === 'admin');

  const { data: session, isLoading: sessionLoading } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => getSession(sessionId),
  });

  const { data: history, isLoading: messagesLoading, isError: messagesError } = useQuery({
    queryKey: ['session-messages', sessionId, offset],
    queryFn: () => listMessages(sessionId, historyPageSize, offset),
    refetchInterval: offset === 0 ? 5000 : false,
  });
  const messages = history?.messages;

  const transcriptExport = useMutation({
    mutationFn: (format: TranscriptFormat) => downloadTranscript(sessionId, format),
    onError: (err) => notifications.show({
      message: isAxiosError(err) && err.response?.status === 422
        ? t('transcriptTooLarge')
        : t('transcriptExportFailed'),
      color: 'red',
    }),
  });

  const statusMutation = useMutation({
    mutationFn: (status: SessionStatus) => updateSessionStatus(sessionId, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] });
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      notifications.show({ message: t('statusUpdated'), color: 'green' });
    },
    onError: () => {
      notifications.show({ message: t('statusUpdateFailed'), color: 'red' });
    },
  });

  if (sessionLoading || !session) {
    return <Loader />;
  }

  return (
    <Stack>
      <Group justify="space-between">
        <Group>
          <Title order={2}>{session.title}</Title>
          <Badge color={statusColors[session.status]}>{t(sessionStatusMessages[session.status])}</Badge>
        </Group>

        <Select
          placeholder={t('changeStatus')}
          data={[
            { value: 'active', label: t('sessionActive') },
            { value: 'paused', label: t('sessionPaused') },
            { value: 'closed', label: t('sessionClosed') },
          ]}
          value={session.status}
          disabled={statusMutation.isPending}
          onChange={(value) => {
            if (value === 'closed' && session.status !== 'closed') {
              setConfirmClose(true);
            } else if (value && value !== session.status) {
              statusMutation.mutate(value as SessionStatus);
            }
          }}
          w={200}
        />
      </Group>
      <Modal opened={confirmClose} onClose={() => setConfirmClose(false)} title={t('closeSessionTitle')}
        closeButtonProps={{ 'aria-label': t('closeDialog') }}>
        <Stack>
          <Text>{t('closeSessionEffect')}</Text>
          <Group justify="end">
            <Button variant="default" onClick={() => setConfirmClose(false)}>{t('cancel')}</Button>
            <Button color="red" onClick={() => {
              setConfirmClose(false);
              statusMutation.mutate('closed');
            }}>{t('closeSession')}</Button>
          </Group>
        </Stack>
      </Modal>

      {isAdmin && <SessionOwner sessionId={sessionId} ownerId={session.owner_operator_id} />}
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">
          {t('invitationLinks')}
        </Text>
        <Stack gap="xs">
          <DeeplinkRow key={`${sessionId}-client`} label={t('client')} sessionId={sessionId} role="client" active={session.status === 'active'} connected={!!session.client_user_id} />
          <DeeplinkRow key={`${sessionId}-executor`} label={t('executor')} sessionId={sessionId} role="executor" active={session.status === 'active'} connected={!!session.executor_user_id} />
        </Stack>
      </Paper>

      <Divider label={t('conversation')} />
      <Group justify="end" gap="xs">
        <Button variant="light" disabled={transcriptExport.isPending} onClick={() => transcriptExport.mutate('csv')}>
          {t('exportTranscriptCSV')}
        </Button>
        <Button variant="light" disabled={transcriptExport.isPending} onClick={() => transcriptExport.mutate('pdf')}>
          {t('exportTranscriptPDF')}
        </Button>
      </Group>

      <Paper withBorder p="md">
        {(offset > 0 || history?.has_more) && (
          <Group justify="end" mb="sm">
            <Button variant="default" disabled={messagesLoading || !history?.has_more} onClick={() => setOffset(offset + historyPageSize)}>{t('olderMessages')}</Button>
            <Button variant="default" disabled={messagesLoading || offset === 0} onClick={() => setOffset(offset - historyPageSize)}>{t('newerMessages')}</Button>
          </Group>
        )}
        {messagesError && <Text c="red">{t('messagesLoadFailed')}</Text>}
        {messagesLoading ? (
          <Loader size="sm" />
        ) : (
          <ScrollArea h={400}>
            <Stack gap="xs">
              {messages?.map((msg) => (
                <MessageRow key={msg.id} message={msg} />
              ))}
              {messages?.length === 0 && (
                <Text c="dimmed" ta="center">
                  {t('noMessages')}
                </Text>
              )}
            </Stack>
          </ScrollArea>
        )}
      </Paper>
      {session.status === 'active' && (
        <ReplyComposer
          key={`${sessionId}:${session.client_user_id}:${session.executor_user_id}`}
          sessionId={sessionId}
          clientUserId={session.client_user_id}
          executorUserId={session.executor_user_id}
          onSend={() => setOffset(0)}
        />
      )}
    </Stack>
  );
}

function SessionOwner({ sessionId, ownerId }: { sessionId: number; ownerId: number | null }) {
  const { t } = usePanelLocale();
  const queryClient = useQueryClient();
  const size = 50;
  const [offset, setOffset] = useState(0);
  const { data, isError, isLoading } = useQuery({
    queryKey: ['operators', offset],
    queryFn: () => listOperators(size, offset),
  });
  const operators = data?.operators;
  const assign = useMutation({
    mutationFn: (operatorId: number) => assignSessionOwner(sessionId, operatorId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] });
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
    onError: () => notifications.show({ message: t('assignOperatorFailed'), color: 'red' }),
  });
  return (
    <Stack gap="xs">
    <Select label={t('assignedOperator')} value={ownerId?.toString() ?? null}
      placeholder={ownerId === null ? undefined : t('operatorNumber', { id: ownerId })}
      error={isError ? t('operatorListLoadFailed') : undefined}
      disabled={assign.isPending || !operators}
      data={(operators ?? []).map(op => ({ value: String(op.id), label: op.name, disabled: op.status !== 'active' }))}
      onChange={value => { if (value && Number(value) !== ownerId) assign.mutate(Number(value)); }}
    />
    <ListPager offset={offset} size={size} hasMore={data?.has_more ?? false} loading={assign.isPending || isLoading} onChange={setOffset} />
    </Stack>
  );
}

function ReplyComposer({ sessionId, clientUserId, executorUserId, onSend }: {
  sessionId: number; clientUserId: number | null; executorUserId: number | null; onSend: () => void;
}) {
  const { t } = usePanelLocale();
  const queryClient = useQueryClient();
  const [recipientRole, setRecipientRole] = useState<OperatorReply['recipientRole'] | null>(null);
  const [text, setText] = useState('');
  const requestKey = useRef<string | null>(null);
  const recipientUserId = recipientRole === 'client' ? clientUserId : recipientRole === 'executor' ? executorUserId : null;
  const send = useMutation({
    mutationFn: (reply: OperatorReply) => sendAsParticipant(sessionId, reply),
    onSuccess: () => {
      requestKey.current = null;
      setText('');
      onSend();
      queryClient.invalidateQueries({ queryKey: ['session-messages', sessionId] });
      notifications.show({ message: t('messageQueued'), color: 'green' });
    },
    onError: (err) => {
      const changed = isAxiosError(err) && err.response?.status === 409;
      if (changed) queryClient.invalidateQueries({ queryKey: ['session', sessionId] });
      notifications.show({ message: t(changed ? 'messageRecipientChanged' : 'messageQueueFailed'), color: 'red' });
    },
  });

  return (
    <form onSubmit={event => {
      event.preventDefault();
      if (!recipientRole || !recipientUserId || !text.trim() || send.isPending) return;
      requestKey.current ??= createOperatorReplyKey();
      send.mutate({ recipientRole, recipientUserId, text: text.trim(), requestKey: requestKey.current });
    }}>
      <Stack gap="xs">
        <Select label={t('messageRecipient')} value={recipientRole} disabled={send.isPending}
          onChange={value => {
            if (value === 'client' || value === 'executor' || value === null) {
              setRecipientRole(value);
              requestKey.current = null;
            }
          }}
          data={[
            { value: 'client', label: t('client'), disabled: !clientUserId },
            { value: 'executor', label: t('executor'), disabled: !executorUserId },
          ]}
        />
        <Textarea label={t('message')} value={text} disabled={send.isPending} maxLength={4000}
          onChange={event => {
            setText(event.currentTarget.value);
            requestKey.current = null;
          }}
        />
        {recipientRole && <Text size="sm">{t('messageRecipientContext', {
          id: sessionId,
          recipient: t(recipientRole),
          sender: t(recipientRole === 'client' ? 'executor' : 'client'),
        })}</Text>}
        <Button type="submit" loading={send.isPending} disabled={!recipientUserId || !text.trim()}>
          {t('send')}
        </Button>
      </Stack>
    </form>
  );
}

function DeeplinkRow({ label, sessionId, role, active, connected }: {
  label: string;
  sessionId: number;
  role: 'client' | 'executor';
  active: boolean;
  connected: boolean;
}) {
  const { t, locale } = usePanelLocale();
  const [invite, setInvite] = useState<{ token: string; expires_at: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const { botUsername } = panelConfig();
  const link = invite && botUsername ? `https://t.me/${botUsername}?start=${invite.token}` : null;

  async function changeInvite(revoke: boolean) {
    setBusy(true);
    try {
      if (revoke) {
        await revokeSessionInvite(sessionId, role);
        setInvite(null);
      } else {
        setInvite(await issueSessionInvite(sessionId, role));
      }
    } catch {
      notifications.show({ message: t('invitationChangeFailed'), color: 'red' });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Group justify="space-between">
      <Group gap="xs">
        <Text size="sm" w={100}>
          {label}:
        </Text>
        <Text size="sm" c="dimmed">
          {connected ? `✓ ${t('connected')}` : t('disconnected')}
        </Text>
      </Group>
      {!connected && (
        <Group gap="xs">
          {active && <Button size="xs" variant="light" disabled={busy || !botUsername} onClick={() => void changeInvite(false)}>
            {t('newInvitation')}
          </Button>}
          {active && !botUsername && <Text size="xs" c="dimmed">{t('botNameMissing')}</Text>}
          <Button size="xs" variant="subtle" disabled={busy} onClick={() => void changeInvite(true)}>
            {t('revoke')}
          </Button>
          {active && invite && link && (
            <Text size="xs">{t('invitationExpires', { date: new Date(invite.expires_at).toLocaleString(locale) })}</Text>
          )}
        </Group>
      )}
      {active && !connected && link && <CopyButton value={link}>
        {({ copied, copy }) => (
          <Tooltip label={copied ? t('copied') : t('copyLink')}>
            <ActionIcon color={copied ? 'teal' : 'blue'} onClick={copy} aria-label={copied ? t('copied') : t('copyLink')}>
              {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
            </ActionIcon>
          </Tooltip>
        )}
      </CopyButton>}
    </Group>
  );
}

import { apiClient } from '../api/client';

function MediaImage({ messageId }: { messageId: number }) {
  const { t } = usePanelLocale();
  const [src, setSrc] = useState<string | null>(null);

  useEffect(() => {
    let objectUrl: string | null = null;

    apiClient
      .get(`/messages/${messageId}/media`, { responseType: 'blob' })
      .then((res) => {
        objectUrl = URL.createObjectURL(res.data);
        setSrc(objectUrl);
      })
      .catch(() => setSrc(null));

    return () => {
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [messageId]);

  if (!src) return <Text size="xs" c="dimmed">{t('loadingPhoto')}</Text>;

  return <img src={src} alt={t('photo')} style={{ maxWidth: 250, borderRadius: 4, display: 'block' }} />;
}

function MediaAudio({ messageId }: { messageId: number }) {
  const { t } = usePanelLocale();
  const [src, setSrc] = useState<string | null>(null);

  useEffect(() => {
    let objectUrl: string | null = null;

    apiClient
      .get(`/messages/${messageId}/media`, { responseType: 'blob' })
      .then((res) => {
        objectUrl = URL.createObjectURL(res.data);
        setSrc(objectUrl);
      })
      .catch(() => setSrc(null));

    return () => {
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [messageId]);

  if (!src) return <Text size="xs" c="dimmed">{t('loadingAudio')}</Text>;

  return <audio controls src={src} style={{ maxWidth: 250 }} />;
}

function MessageRow({ message }: { message: import('../types/api').Message }) {
  const { t, locale } = usePanelLocale();
  const role = message.impersonated_role ?? message.sender_role;
  const senderLabel = role === 'client' ? t('client') : t('executor');
  const align = role === 'client' ? 'flex-start' : 'flex-end';

  return (
    <Group justify={align === 'flex-start' ? 'flex-start' : 'flex-end'}>
      <Paper p="xs" withBorder maw="70%" bg={message.sender_role === 'client' ? 'gray.0' : 'blue.0'}>
        <Text size="xs" fw={600} c="dimmed">
          {message.sent_by_operator ? t('sentByOperator', { role: senderLabel.toLocaleLowerCase(locale) }) : senderLabel}
        </Text>
        {message.content_type === 'photo' && <MediaImage messageId={message.id} />}
        {message.content_type === 'voice' && <MediaAudio messageId={message.id} />}
        {message.content && <Text size="sm">{message.content}</Text>}
        <Text size="xs" c="dimmed" mt={4}>
          {new Date(message.created_at).toLocaleTimeString(locale)}
          {message.delivery_status === 'failed' && ` · ${t('deliveryFailed')}`}
          {message.delivery_status === 'retrying' && ` · ${t('deliveryRetry')}`}
          {message.delivery_status === 'queued' && ` · ${t('deliveryQueued')}`}
          {message.delivery_status === 'delivered' && message.delivered && ` · ${t('delivered')}`}
          {message.delivery_status === 'legacy_unknown' && ` · ${t('deliveryUnknown')}`}
        </Text>
      </Paper>
    </Group>
  );
}
