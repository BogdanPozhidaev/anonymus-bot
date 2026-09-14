import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Table, Title, Button, Group, TextInput, Loader, Text, Code } from '@mantine/core';
import { IconDownload } from '@tabler/icons-react';
import { notifications } from '@mantine/notifications';

import { listAuditLog, downloadAuditLogCsv, type AuditLogFilters } from '../api/auditLog';

export function AuditLogPage() {
  const [filters, setFilters] = useState<AuditLogFilters>({});
  const [actionFilter, setActionFilter] = useState('');

  const { data: entries, isLoading } = useQuery({
    queryKey: ['audit-log', filters],
    queryFn: () => listAuditLog(filters),
  });

  const handleFilterApply = () => {
    setFilters({ action: actionFilter || undefined });
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
          Экспорт CSV
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

      {isLoading ? (
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
                <Table.Td>{entry.actor_id ?? '—'}</Table.Td>
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

      {entries?.length === 0 && (
        <Text c="dimmed" ta="center" mt="xl">
          Записей не найдено
        </Text>
      )}
    </>
  );
}