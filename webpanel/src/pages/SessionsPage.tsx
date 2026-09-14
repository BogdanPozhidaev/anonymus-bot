import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Table,
  Badge,
  Button,
  Group,
  Title,
  Modal,
  TextInput,
  Stack,
  Loader,
  Text,
  ActionIcon,
} from '@mantine/core';
import { useForm } from '@mantine/form';
import { notifications } from '@mantine/notifications';
import { IconPlus, IconExternalLink } from '@tabler/icons-react';

import { listSessions, createSession } from '../api/sessions';
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

export function SessionsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [createModalOpen, setCreateModalOpen] = useState(false);

  const { data: sessions, isLoading, error } = useQuery({
    queryKey: ['sessions'],
    queryFn: listSessions,
  });

  const createForm = useForm({
    initialValues: { title: '' },
  });

  const createMutation = useMutation({
    mutationFn: (title: string) => createSession(title),
    onSuccess: (newSession) => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
      notifications.show({ message: 'Сессия создана', color: 'green' });
      setCreateModalOpen(false);
      createForm.reset();
      navigate(`/sessions/${newSession.id}`);
    },
    onError: () => {
      notifications.show({ message: 'Не удалось создать сессию', color: 'red' });
    },
  });

  if (isLoading) {
    return <Loader />;
  }

  if (error) {
    return <Text c="red">Не удалось загрузить список сессий</Text>;
  }

  return (
    <>
      <Group justify="space-between" mb="md">
        <Title order={2}>Сессии</Title>
        <Button leftSection={<IconPlus size={18} />} onClick={() => setCreateModalOpen(true)}>
          Создать сессию
        </Button>
      </Group>

      <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>ID</Table.Th>
            <Table.Th>Название</Table.Th>
            <Table.Th>Статус</Table.Th>
            <Table.Th>Клиент</Table.Th>
            <Table.Th>Исполнитель</Table.Th>
            <Table.Th>Создана</Table.Th>
            <Table.Th></Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {sessions?.map((session) => (
            <Table.Tr key={session.id}>
              <Table.Td>{session.id}</Table.Td>
              <Table.Td>{session.title}</Table.Td>
              <Table.Td>
                <Badge color={statusColors[session.status]}>
                  {statusLabels[session.status]}
                </Badge>
              </Table.Td>
              <Table.Td>{session.client_user_id ? 'Подключён' : '—'}</Table.Td>
              <Table.Td>{session.executor_user_id ? 'Подключён' : '—'}</Table.Td>
              <Table.Td>{new Date(session.created_at).toLocaleString('ru-RU')}</Table.Td>
              <Table.Td>
                <ActionIcon variant="subtle" onClick={() => navigate(`/sessions/${session.id}`)}>
                  <IconExternalLink size={18} />
                </ActionIcon>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>

      {sessions?.length === 0 && (
        <Text c="dimmed" ta="center" mt="xl">
          Сессий пока нет
        </Text>
      )}

      <Modal
        opened={createModalOpen}
        onClose={() => setCreateModalOpen(false)}
        title="Новая сессия"
      >
        <form onSubmit={createForm.onSubmit((values) => createMutation.mutate(values.title))}>
          <Stack>
            <TextInput
              label="Внутреннее название"
              placeholder="Например: Заказ №123"
              required
              {...createForm.getInputProps('title')}
            />
            <Button type="submit" loading={createMutation.isPending}>
              Создать
            </Button>
          </Stack>
        </form>
      </Modal>
    </>
  );
}