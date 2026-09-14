import { useParams } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Title,
  Badge,
  Group,
  Paper,
  Stack,
  Text,
  CopyButton,
  Tooltip,
  ActionIcon,
  Divider,
  ScrollArea,
  Loader,
  Select,
} from '@mantine/core';
import { IconCopy, IconCheck } from '@tabler/icons-react';
import { notifications } from '@mantine/notifications';

import { getSession, updateSessionStatus } from '../api/sessions';
import { listMessages } from '../api/messages';
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

const BOT_USERNAME = 'test_fastcheck_bot';

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const sessionId = Number(id);
  const queryClient = useQueryClient();

  const { data: session, isLoading: sessionLoading } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => getSession(sessionId),
  });

  const { data: messages, isLoading: messagesLoading } = useQuery({
    queryKey: ['session-messages', sessionId],
    queryFn: () => listMessages(sessionId),
    refetchInterval: 5000, // просто периодический опрос для MVP, без WebSocket
  });

  const statusMutation = useMutation({
    mutationFn: (status: SessionStatus) => updateSessionStatus(sessionId, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['session', sessionId] });
      notifications.show({ message: 'Статус обновлён', color: 'green' });
    },
    onError: () => {
      notifications.show({ message: 'Не удалось обновить статус', color: 'red' });
    },
  });

  if (sessionLoading || !session) {
    return <Loader />;
  }

  const clientLink = `https://t.me/${BOT_USERNAME}?start=${sessionId}-c`;
  const executorLink = `https://t.me/${BOT_USERNAME}?start=${sessionId}-e`;

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
          onChange={(value) => value && statusMutation.mutate(value as SessionStatus)}
          w={200}
        />
      </Group>

      <Paper withBorder p="md">
        <Text fw={600} mb="sm">
          Ссылки для подключения
        </Text>
        <Stack gap="xs">
          <DeeplinkRow label="Клиент" link={clientLink} connected={!!session.client_user_id} />
          <DeeplinkRow label="Исполнитель" link={executorLink} connected={!!session.executor_user_id} />
        </Stack>
      </Paper>

      <Divider label="Переписка" />

      <Paper withBorder p="md">
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
    </Stack>
  );
}

function DeeplinkRow({ label, link, connected }: { label: string; link: string; connected: boolean }) {
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
      <CopyButton value={link}>
        {({ copied, copy }) => (
          <Tooltip label={copied ? 'Скопировано' : 'Скопировать ссылку'}>
            <ActionIcon color={copied ? 'teal' : 'blue'} onClick={copy}>
              {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
            </ActionIcon>
          </Tooltip>
        )}
      </CopyButton>
    </Group>
  );
}

function MessageRow({ message }: { message: import('../types/api').Message }) {
  const senderLabel = message.sender_role === 'client' ? 'Клиент' : 'Менеджер';
  const align = message.sender_role === 'client' ? 'flex-start' : 'flex-end';

  return (
    <Group justify={align === 'flex-start' ? 'flex-start' : 'flex-end'}>
      <Paper p="xs" withBorder maw="70%" bg={message.sender_role === 'client' ? 'gray.0' : 'blue.0'}>
        <Text size="xs" fw={600} c="dimmed">
          {senderLabel}
        </Text>
        {message.content_type === 'text' && <Text size="sm">{message.content}</Text>}
        {message.content_type === 'photo' && <Text size="sm" fs="italic">[Фото]</Text>}
        {message.content_type === 'voice' && <Text size="sm" fs="italic">[Голосовое сообщение]</Text>}
        <Text size="xs" c="dimmed" mt={4}>
          {new Date(message.created_at).toLocaleTimeString('ru-RU')}
          {!message.delivered && ' · не доставлено'}
        </Text>
      </Paper>
    </Group>
  );
}