import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Table, Title, Button, Group, TextInput, Loader, Text, Code, Alert } from '@mantine/core';
import { IconDownload } from '@tabler/icons-react';
import { notifications } from '@mantine/notifications';

import { listAuditLog, downloadAuditLogCsv, type AuditLogFilters } from '../api/auditLog';

export function AuditLogPage() {
  const pageSize = 100;
  const [filters, setFilters] = useState<AuditLogFilters>({ limit: pageSize, offset: 0 });
  const [actionFilter, setActionFilter] = useState('');

  const { data, isLoading, isError } = useQuery({
    queryKey: ['audit-log', filters],
    queryFn: () => listAuditLog(filters),
  });

  const entries = data?.entries;
  const offset = filters.offset ?? 0;

  const handleFilterApply = () => {
    setFilters({ action: actionFilter || undefined, limit: pageSize, offset: 0 });
  };

  const handleExport = async () => {
    try {
      await downloadAuditLogCsv(filters);
    } catch {
      notifications.show({ message: 'Не удалось экспортировать', color: 'red' });
    }
  };

  return (
    <>
      <Group justify="space-between" mb="md">
        <Title order={2}>Аудит действий</Title>
        <Button leftSection={<IconDownload size={18} />} onClick={handleExport} variant="light">
          Экспорт страницы CSV
        </Button>
      </Group>

      <Group mb="md">
        <TextInput
          placeholder="Фильтр по действию (например, session_status_updated)"
          value={actionFilter}
          onChange={(e) => setActionFilter(e.currentTarget.value)}
          w={350}
        />
        <Button onClick={handleFilterApply} variant="default">
          Применить
        </Button>
      </Group>

      {isError ? (
        <Alert color="red">Не удалось загрузить журнал</Alert>
      ) : isLoading ? (
        <Loader />
      ) : (
        <Table striped highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Время</Table.Th>
              <Table.Th>Actor ID</Table.Th>
              <Table.Th>Действие</Table.Th>
              <Table.Th>Цель</Table.Th>
              <Table.Th>Детали</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {entries?.map((entry) => (
              <Table.Tr key={entry.id}>
                <Table.Td>{new Date(entry.timestamp).toLocaleString('ru-RU')}</Table.Td>
                <Table.Td>{entry.actor_id == null ? entry.actor_type : `${entry.actor_type} #${entry.actor_id}`}</Table.Td>
                <Table.Td>{entry.action}</Table.Td>
                <Table.Td>
                  {entry.target_type ? `${entry.target_type} #${entry.target_id}` : '—'}
                </Table.Td>
                <Table.Td>
                  {entry.payload ? (
                    <Code block style={{ fontSize: 11 }}>
                      {JSON.stringify(entry.payload)}
                    </Code>
                  ) : (
                    '—'
                  )}
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}

      <Group mt="md">
        <Button variant="default" disabled={offset === 0 || isLoading} onClick={() => setFilters({ ...filters, offset: offset - pageSize })}>
          Назад
        </Button>
        <Button variant="default" disabled={!data?.has_more || offset >= 1000000} onClick={() => setFilters({ ...filters, offset: offset + pageSize })}>
          Далее
        </Button>
      </Group>

      {entries?.length === 0 && (
        <Text c="dimmed" ta="center" mt="xl">
          Записей не найдено
        </Text>
      )}
    </>
  );
}