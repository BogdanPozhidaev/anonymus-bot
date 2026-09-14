import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Table,
  Title,
  Button,
  Badge,
  Group,
  Modal,
  TextInput,
  Select,
  Stack,
  Loader,
  Text,
  Alert,
  CopyButton,
  ActionIcon,
  Tooltip,
} from '@mantine/core';
import { useForm } from '@mantine/form';
import { notifications } from '@mantine/notifications';
import { IconPlus, IconCopy, IconCheck } from '@tabler/icons-react';

import { listOperators, createOperator, updateOperator } from '../api/operators';
import type { OperatorRole, OperatorStatus } from '../types/api';

const statusColors: Record<OperatorStatus, string> = {
  invited: 'blue',
  active: 'green',
  suspended: 'yellow',
  disabled: 'red',
};

const statusLabels: Record<OperatorStatus, string> = {
  invited: 'Приглашён',
  active: 'Активен',
  suspended: 'Приостановлен',
  disabled: 'Отключён',
};

export function OperatorsPage() {
  const queryClient = useQueryClient();
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createdCredentials, setCreatedCredentials] = useState<{ login: string; password: string } | null>(null);

  const { data: operators, isLoading } = useQuery({
    queryKey: ['operators'],
    queryFn: listOperators,
  });

  const createForm = useForm({
    initialValues: { name: '', login: '', email: '', role: 'operator' as OperatorRole },
  });

  const createMutation = useMutation({
    mutationFn: (values: typeof createForm.values) =>
      createOperator(values.name, values.login, values.role, values.email || undefined),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['operators'] });
      setCreatedCredentials({ login: result.login, password: result.temporary_password });
      createForm.reset();
    },
    onError: () => {
      notifications.show({ message: 'Не удалось создать оператора', color: 'red' });
    },
  });

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: OperatorStatus }) =>
      updateOperator(id, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['operators'] });
      notifications.show({ message: 'Статус обновлён', color: 'green' });
    },
    onError: () => {
      notifications.show({ message: 'Не удалось обновить статус', color: 'red' });
    },
  });

  const handleCloseCreateModal = () => {
    setCreateModalOpen(false);
    setCreatedCredentials(null);
  };

  if (isLoading) {
    return <Loader />;
  }

  return (
    <>
      <Group justify="space-between" mb="md">
        <Title order={2}>Операторы</Title>
        <Button leftSection={<IconPlus size={18} />} onClick={() => setCreateModalOpen(true)}>
          Добавить оператора
        </Button>
      </Group>

      <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Имя</Table.Th>
            <Table.Th>Логин</Table.Th>
            <Table.Th>Роль</Table.Th>
            <Table.Th>Статус</Table.Th>
            <Table.Th>Последний вход</Table.Th>
            <Table.Th></Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {operators?.map((op) => (
            <Table.Tr key={op.id}>
              <Table.Td>{op.name}</Table.Td>
              <Table.Td>{op.login}</Table.Td>
              <Table.Td>{op.role === 'admin' ? 'Админ' : 'Оператор'}</Table.Td>
              <Table.Td>
                <Badge color={statusColors[op.status]}>{statusLabels[op.status]}</Badge>
              </Table.Td>
              <Table.Td>
                {op.last_login_at ? new Date(op.last_login_at).toLocaleString('ru-RU') : 'Никогда'}
              </Table.Td>
              <Table.Td>
                <Select
                  size="xs"
                  w={150}
                  placeholder="Изменить статус"
                  data={[
                    { value: 'active', label: 'Активен' },
                    { value: 'suspended', label: 'Приостановить' },
                    { value: 'disabled', label: 'Отключить' },
                  ]}
                  onChange={(value) =>
                    value && statusMutation.mutate({ id: op.id, status: value as OperatorStatus })
                  }
                />
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>

      <Modal opened={createModalOpen} onClose={handleCloseCreateModal} title="Новый оператор">
        {createdCredentials ? (
          <Stack>
            <Alert color="green" title="Оператор создан">
              Передайте эти данные оператору лично — пароль больше нигде не отобразится.
            </Alert>
            <Group>
              <Text size="sm">Логин: {createdCredentials.login}</Text>
            </Group>
            <Group>
              <Text size="sm">Временный пароль: {createdCredentials.password}</Text>
              <CopyButton value={createdCredentials.password}>
                {({ copied, copy }) => (
                  <Tooltip label={copied ? 'Скопировано' : 'Скопировать'}>
                    <ActionIcon onClick={copy} color={copied ? 'teal' : 'blue'}>
                      {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
                    </ActionIcon>
                  </Tooltip>
                )}
              </CopyButton>
            </Group>
            <Button onClick={handleCloseCreateModal}>Готово</Button>
          </Stack>
        ) : (
          <form onSubmit={createForm.onSubmit((values) => createMutation.mutate(values))}>
            <Stack>
              <TextInput label="Имя" required {...createForm.getInputProps('name')} />
              <TextInput label="Логин" required {...createForm.getInputProps('login')} />
              <TextInput label="Email (опционально)" {...createForm.getInputProps('email')} />
              <Select
                label="Роль"
                data={[
                  { value: 'operator', label: 'Оператор' },
                  { value: 'admin', label: 'Админ' },
                ]}
                {...createForm.getInputProps('role')}
              />
              <Button type="submit" loading={createMutation.isPending}>
                Создать
              </Button>
            </Stack>
          </form>
        )}
      </Modal>
    </>
  );
}