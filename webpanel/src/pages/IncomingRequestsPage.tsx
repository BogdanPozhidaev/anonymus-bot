import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Table, Title, Button, Text, Loader, Badge } from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { listIncomingRequests, markIncomingRequestProcessed } from '../api/incomingRequests';
import { createSession } from '../api/sessions';

export function IncomingRequestsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: requests, isLoading } = useQuery({
    queryKey: ['incoming-requests'],
    queryFn: () => listIncomingRequests('new'),
    refetchInterval: 10000,
  });

  const createSessionMutation = useMutation({
    mutationFn: async (requestId: number) => {
      const request = requests?.find((r) => r.id === requestId);
      const title = request?.first_name
        ? `Обращение от ${request.first_name}`
        : `Обращение #${requestId}`;

      const session = await createSession(title);
      await markIncomingRequestProcessed(requestId);
      return session;
    },
    onSuccess: (session) => {
      queryClient.invalidateQueries({ queryKey: ['incoming-requests'] });
      notifications.show({ message: 'Сессия создана', color: 'green' });
      navigate(`/sessions/${session.id}`);
    },
    onError: () => {
      notifications.show({ message: 'Не удалось создать сессию', color: 'red' });
    },
  });

  if (isLoading) {
    return <Loader />;
  }

  return (
    <>
      <Title order={2} mb="md">
        Входящие обращения
      </Title>

      <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Имя</Table.Th>
            <Table.Th>Username</Table.Th>
            <Table.Th>Первое сообщение</Table.Th>
            <Table.Th>Язык</Table.Th>
            <Table.Th>Дата</Table.Th>
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
              <Table.Td>{new Date(req.created_at).toLocaleString('ru-RU')}</Table.Td>
              <Table.Td>
                <Button
                  size="xs"
                  onClick={() => createSessionMutation.mutate(req.id)}
                  loading={createSessionMutation.isPending}
                >
                  Создать сессию
                </Button>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>

      {requests?.length === 0 && (
        <Text c="dimmed" ta="center" mt="xl">
          Новых обращений нет
        </Text>
      )}
    </>
  );
}