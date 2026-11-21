import { useParams } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
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
import { listMessages, sendAsParticipant } from '../api/messages';
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

const statusLabels: Record<SessionStatus, string> = {
  active: 'Активна',
  paused: 'На паузе',
  pending_close: 'Ожидает закрытия',
  closed: 'Закрыта',
};

const historyPageSize = 50;

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  return <SessionDetail key={id} sessionId={Number(id)} />;
}

function SessionDetail({ sessionId }: { sessionId: number }) {
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

  const statusMutation = useMutation({
    mutationFn: (status: SessionStatus) => updateSessionStatus(sessionId, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] });
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      notifications.show({ message: 'Статус обновлён', color: 'green' });
    },
    onError: () => {
      notifications.show({ message: 'Не удалось обновить статус', color: 'red' });
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
          <Badge color={statusColors[session.status]}>{statusLabels[session.status]}</Badge>
        </Group>

        <Select
          placeholder="Изменить статус"
          data={[
            { value: 'active', label: 'Активна' },
            { value: 'paused', label: 'На паузе' },
            { value: 'closed', label: 'Закрыта' },
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
      <Modal opened={confirmClose} onClose={() => setConfirmClose(false)} title="Закрыть сессию?">
        <Stack>
          <Text>Участники больше не смогут отправлять сообщения в этой сессии.</Text>
          <Group justify="end">
            <Button variant="default" onClick={() => setConfirmClose(false)}>Отмена</Button>
            <Button color="red" onClick={() => {
              setConfirmClose(false);
              statusMutation.mutate('closed');
            }}>Закрыть сессию</Button>
          </Group>
        </Stack>
      </Modal>

      {isAdmin && <SessionOwner sessionId={sessionId} ownerId={session.owner_operator_id} />}
      <Paper withBorder p="md">
        <Text fw={600} mb="sm">
          Ссылки для подключения
        </Text>
        <Stack gap="xs">
          <DeeplinkRow key={`${sessionId}-client`} label="Клиент" sessionId={sessionId} role="client" active={session.status === 'active'} connected={!!session.client_user_id} />
          <DeeplinkRow key={`${sessionId}-executor`} label="Исполнитель" sessionId={sessionId} role="executor" active={session.status === 'active'} connected={!!session.executor_user_id} />
        </Stack>
      </Paper>

      <Divider label="Переписка" />

      <Paper withBorder p="md">
        {(offset > 0 || history?.has_more) && (
          <Group justify="end" mb="sm">
            <Button variant="default" disabled={messagesLoading || !history?.has_more} onClick={() => setOffset(offset + historyPageSize)}>Ранние</Button>
            <Button variant="default" disabled={messagesLoading || offset === 0} onClick={() => setOffset(offset - historyPageSize)}>Новые</Button>
          </Group>
        )}
        {messagesError && <Text c="red">Не удалось загрузить сообщения</Text>}
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
                  Сообщений пока нет
                </Text>
              )}
            </Stack>
          </ScrollArea>
        )}
      </Paper>
      {session.status === 'active' && (
        <ReplyComposer
          sessionId={sessionId}
          clientConnected={session.client_user_id !== null}
          executorConnected={session.executor_user_id !== null}
          onSend={() => setOffset(0)}
        />
      )}
    </Stack>
  );
}

function SessionOwner({ sessionId, ownerId }: { sessionId: number; ownerId: number | null }) {
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
    onError: () => notifications.show({ message: 'Не удалось назначить оператора', color: 'red' }),
  });
  return (
    <Stack gap="xs">
    <Select label="Ответственный оператор" value={ownerId?.toString() ?? null}
      placeholder={ownerId === null ? undefined : `Оператор #${ownerId}`}
      error={isError ? 'Не удалось загрузить список операторов' : undefined}
      disabled={assign.isPending || !operators}
      data={(operators ?? []).map(op => ({ value: String(op.id), label: op.name, disabled: op.status !== 'active' }))}
      onChange={value => { if (value && Number(value) !== ownerId) assign.mutate(Number(value)); }}
    />
    <ListPager offset={offset} size={size} hasMore={data?.has_more ?? false} loading={assign.isPending || isLoading} onChange={setOffset} />
    </Stack>
  );
}

function ReplyComposer({ sessionId, clientConnected, executorConnected, onSend }: {
  sessionId: number; clientConnected: boolean; executorConnected: boolean; onSend: () => void;
}) {
  const queryClient = useQueryClient();
  const [role, setRole] = useState<'client' | 'executor' | null>(null);
  const [text, setText] = useState('');
  const recipientConnected = role === 'client' ? executorConnected : role === 'executor' && clientConnected;
  const send = useMutation({
    mutationFn: ({ role, text }: { role: 'client' | 'executor'; text: string }) => sendAsParticipant(sessionId, role, text),
    onSuccess: () => {
      setText('');
      onSend();
      queryClient.invalidateQueries({ queryKey: ['session-messages', sessionId] });
      notifications.show({ message: 'Сообщение поставлено в очередь', color: 'green' });
    },
    onError: () => notifications.show({ message: 'Не удалось поставить сообщение в очередь', color: 'red' }),
  });

  return (
    <form onSubmit={event => {
      event.preventDefault();
      if (role && recipientConnected && text.trim() && !send.isPending) send.mutate({ role, text: text.trim() });
    }}>
      <Stack gap="xs">
        <Select label="Отправить от имени" value={role} disabled={send.isPending}
          onChange={value => { if (value === 'client' || value === 'executor') setRole(value); }}
          data={[
            { value: 'client', label: 'Клиент', disabled: !executorConnected },
            { value: 'executor', label: 'Исполнитель', disabled: !clientConnected },
          ]}
        />
        <Textarea label="Сообщение" value={text} disabled={send.isPending} maxLength={4000}
          onChange={event => setText(event.currentTarget.value)}
        />
        <Button type="submit" loading={send.isPending} disabled={!role || !recipientConnected || !text.trim()}>
          Отправить
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
      notifications.show({ message: 'Не удалось изменить приглашение', color: 'red' });
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
          {connected ? '✓ Подключён' : 'Не подключён'}
        </Text>
      </Group>
      {!connected && (
        <Group gap="xs">
          {active && <Button size="xs" variant="light" disabled={busy || !botUsername} onClick={() => void changeInvite(false)}>
            Выпустить новую ссылку
          </Button>}
          {active && !botUsername && <Text size="xs" c="dimmed">Имя бота не настроено</Text>}
          <Button size="xs" variant="subtle" disabled={busy} onClick={() => void changeInvite(true)}>
            Отозвать
          </Button>
          {active && invite && link && (
            <Text size="xs">До {new Date(invite.expires_at).toLocaleString()}</Text>
          )}
        </Group>
      )}
      {active && !connected && link && <CopyButton value={link}>
        {({ copied, copy }) => (
          <Tooltip label={copied ? 'Скопировано' : 'Скопировать ссылку'}>
            <ActionIcon color={copied ? 'teal' : 'blue'} onClick={copy}>
              {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
            </ActionIcon>
          </Tooltip>
        )}
      </CopyButton>}
    </Group>
  );
}

import { useEffect, useState } from 'react';
import { apiClient } from '../api/client';

function MediaImage({ messageId }: { messageId: number }) {
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

  if (!src) return <Text size="xs" c="dimmed">Загрузка фото…</Text>;

  return <img src={src} alt="Фото" style={{ maxWidth: 250, borderRadius: 4, display: 'block' }} />;
}

function MediaAudio({ messageId }: { messageId: number }) {
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

  if (!src) return <Text size="xs" c="dimmed">Загрузка аудио…</Text>;

  return <audio controls src={src} style={{ maxWidth: 250 }} />;
}

function MessageRow({ message }: { message: import('../types/api').Message }) {
  const role = message.impersonated_role ?? message.sender_role;
  const senderLabel = role === 'client' ? 'Клиент' : 'Исполнитель';
  const align = role === 'client' ? 'flex-start' : 'flex-end';

  return (
    <Group justify={align === 'flex-start' ? 'flex-start' : 'flex-end'}>
      <Paper p="xs" withBorder maw="70%" bg={message.sender_role === 'client' ? 'gray.0' : 'blue.0'}>
        <Text size="xs" fw={600} c="dimmed">
          {message.sent_by_operator ? `Оператор от имени: ${senderLabel.toLowerCase()}` : senderLabel}
        </Text>
        {message.content_type === 'photo' && <MediaImage messageId={message.id} />}
        {message.content_type === 'voice' && <MediaAudio messageId={message.id} />}
        {message.content && <Text size="sm">{message.content}</Text>}
        <Text size="xs" c="dimmed" mt={4}>
          {new Date(message.created_at).toLocaleTimeString('ru-RU')}
          {message.delivery_status === 'failed' && ' · ошибка доставки'}
          {message.delivery_status === 'retrying' && ' · повторная попытка'}
          {message.delivery_status === 'queued' && ' · ожидает доставки'}
          {message.delivery_status === 'delivered' && message.delivered && ' · доставлено'}
          {message.delivery_status === 'legacy_unknown' && ' · доставка не подтверждена'}
        </Text>
      </Paper>
    </Group>
  );
}
