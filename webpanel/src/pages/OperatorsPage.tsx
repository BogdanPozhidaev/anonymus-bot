import { usePanelLocale } from '../i18n/panelLocale';
import type { MessageKey } from '../i18n/messages';
import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
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

import { listOperators, createOperator, updateOperator, resetOperatorPassword } from '../api/operators';
import type { OperatorListItem, OperatorRole, OperatorStatus } from '../types/api';
import { ListPager } from '../components/ListPager';
import { useAuthStore } from '../stores/authStore';

const pageSize = 50;

const statusColors: Record<OperatorStatus, string> = {
  invited: 'blue',
  active: 'green',
  suspended: 'yellow',
  disabled: 'red',
};

const statusLabels: Record<OperatorStatus, MessageKey> = {
  invited: 'operatorInvited',
  active: 'operatorActive',
  suspended: 'operatorSuspended',
  disabled: 'operatorDisabled',
};

function lastAdminConflict(error: unknown) {
  return isAxiosError(error) && error.response?.status === 409;
}

export function OperatorsPage() {
  const { t, locale } = usePanelLocale();
  const queryClient = useQueryClient();
  const currentOperator = useAuthStore(state => state.operatorId);
  const logout = useAuthStore(state => state.logout);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createdCredentials, setCreatedCredentials] = useState<{ login: string; password: string } | null>(null);
  const [resetTarget, setResetTarget] = useState<OperatorListItem | null>(null);
  const [resetPassword, setResetPassword] = useState<string | null>(null);
  const [resetPending, setResetPending] = useState(false);
  const [resetFailed, setResetFailed] = useState(false);
  const [roleTarget, setRoleTarget] = useState<OperatorListItem | null>(null);
  const [selectedRole, setSelectedRole] = useState<OperatorRole>('operator');

  const [offset, setOffset] = useState(0);
  const { data, isLoading, isError } = useQuery({
    queryKey: ['operators', offset],
    queryFn: () => listOperators(pageSize, offset),
  });
  const operators = data?.operators;

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
      notifications.show({ message: t('operatorCreateFailed'), color: 'red' });
    },
  });

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: OperatorStatus }) =>
      updateOperator(id, { status }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['operators'] });
      notifications.show({ message: t('statusUpdated'), color: 'green' });
    },
    onError: error => {
      notifications.show({ message: t(lastAdminConflict(error) ? 'lastActiveAdmin' : 'statusUpdateFailed'), color: 'red' });
    },
  });

  const roleMutation = useMutation({
    mutationFn: ({ id, role }: { id: number; role: OperatorRole }) => updateOperator(id, { role }),
    onSuccess: (_result, { id }) => {
      setRoleTarget(null);
      notifications.show({ message: t('roleUpdated'), color: 'green' });
      if (id === currentOperator) {
        queryClient.clear();
        logout();
      } else {
        void queryClient.invalidateQueries({ queryKey: ['operators'] });
      }
    },
  });

  function openRoleEditor(operator: OperatorListItem) {
    roleMutation.reset();
    setSelectedRole(operator.role);
    setRoleTarget(operator);
  }

  const handleCloseCreateModal = () => {
    setCreateModalOpen(false);
    setCreatedCredentials(null);
  };

  function closeReset() {
    if (resetPending) return;
    setResetTarget(null);
    setResetPassword(null);
    setResetFailed(false);
  }

  async function confirmReset() {
    if (!resetTarget || resetPending || resetPassword) return;
    setResetPending(true);
    setResetFailed(false);
    try {
      const result = await resetOperatorPassword(resetTarget.id);
      setResetPassword(result.temporary_password);
    } catch {
      setResetFailed(true);
    } finally {
      setResetPending(false);
    }
  }

  return (
    <>
      <Group justify="space-between" mb="md">
        <Title order={2}>{t('operators')}</Title>
        <Button leftSection={<IconPlus size={18} />} onClick={() => setCreateModalOpen(true)}>
          {t('addOperator')}
        </Button>
      </Group>

      {isLoading && <Loader />}
      {isError && <Alert color="red">{t('operatorsLoadFailed')}</Alert>}
      <Table striped highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>{t('name')}</Table.Th>
            <Table.Th>{t('login')}</Table.Th>
            <Table.Th>{t('role')}</Table.Th>
            <Table.Th>{t('status')}</Table.Th>
            <Table.Th>{t('lastLogin')}</Table.Th>
            <Table.Th></Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {operators?.map((op) => (
            <Table.Tr key={op.id}>
              <Table.Td>{op.name}</Table.Td>
              <Table.Td>{op.login}</Table.Td>
              <Table.Td>{op.role === 'admin' ? t('admin') : t('operator')}</Table.Td>
              <Table.Td>
                <Badge color={statusColors[op.status]}>{t(statusLabels[op.status])}</Badge>
              </Table.Td>
              <Table.Td>
                {op.last_login_at ? new Date(op.last_login_at).toLocaleString(locale) : t('never')}
              </Table.Td>
              <Table.Td>
                <Stack gap="xs" align="start">
                <Select
                  size="xs"
                  w={150}
                  placeholder={t('changeStatus')}
                  data={[
                    { value: 'active', label: t('operatorActive') },
                    { value: 'suspended', label: t('suspend') },
                    { value: 'disabled', label: t('disable') },
                  ]}
                  onChange={(value) =>
                    value && statusMutation.mutate({ id: op.id, status: value as OperatorStatus })
                  }
                />
                <Button size="xs" variant="subtle" onClick={() => openRoleEditor(op)}>{t('changeRole')}</Button>
                {op.id !== currentOperator && (op.status === 'active' || op.status === 'invited') && (
                  <Button size="xs" variant="subtle" onClick={() => setResetTarget(op)}>
                    {t('resetPassword')}
                  </Button>
                )}
                </Stack>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
      <ListPager offset={offset} size={pageSize} hasMore={data?.has_more ?? false} loading={isLoading} onChange={setOffset} />

      <Modal opened={roleTarget !== null} onClose={() => setRoleTarget(null)}
        title={roleTarget ? t('changeRoleFor', { login: roleTarget.login }) : t('changeRole')}
        closeButtonProps={{ 'aria-label': t('closeDialog') }}
        closeOnClickOutside={!roleMutation.isPending} closeOnEscape={!roleMutation.isPending} withCloseButton={!roleMutation.isPending}>
        {roleTarget && (
          <form onSubmit={event => {
            event.preventDefault();
            if (selectedRole !== roleTarget.role && !roleMutation.isPending) {
              roleMutation.mutate({ id: roleTarget.id, role: selectedRole });
            }
          }}>
            <Stack>
              <Text>{t('roleChangeEffect')}</Text>
              {roleMutation.isError && (
                <Alert role="alert" color="red">
                  {t(lastAdminConflict(roleMutation.error) ? 'lastActiveAdmin' : 'roleUpdateFailed')}
                </Alert>
              )}
              <Select label={t('role')} value={selectedRole} disabled={roleMutation.isPending}
                data={[{ value: 'operator', label: t('operator') }, { value: 'admin', label: t('admin') }]}
                onChange={role => { if (role === 'admin' || role === 'operator') setSelectedRole(role); }} />
              <Group justify="end">
                <Button variant="default" disabled={roleMutation.isPending} onClick={() => setRoleTarget(null)}>{t('cancel')}</Button>
                <Button type="submit" loading={roleMutation.isPending} disabled={selectedRole === roleTarget.role}>{t('confirm')}</Button>
              </Group>
            </Stack>
          </form>
        )}
      </Modal>

      <Modal opened={resetTarget !== null} onClose={closeReset} title={t('resetPasswordTitle')}
        closeButtonProps={{ 'aria-label': t('closeDialog') }}
        closeOnClickOutside={!resetPending} closeOnEscape={!resetPending} withCloseButton={!resetPending}>
        <Stack>
          {resetTarget && <Text>{t('operatorUsername', { login: resetTarget.login })}</Text>}
          {resetPassword ? (
            <>
              <Alert color="green">{t('shareTemporaryPassword')}</Alert>
              <Text style={{ overflowWrap: 'anywhere' }}>{resetPassword}</Text>
              <CopyButton value={resetPassword}>
                {({ copied, copy }) => <Button variant="light" onClick={copy}>{copied ? t('copied') : t('copyPassword')}</Button>}
              </CopyButton>
              <Button onClick={closeReset}>{t('done')}</Button>
            </>
          ) : (
            <>
              <Text>{t('resetPasswordEffect')}</Text>
              {resetFailed && <Alert color="red">{t('resetPasswordFailed')}</Alert>}
              <Group justify="end">
                <Button variant="default" disabled={resetPending} onClick={closeReset}>{t('cancel')}</Button>
                <Button color="red" loading={resetPending} onClick={() => void confirmReset()}>{t('issueTemporaryPassword')}</Button>
              </Group>
            </>
          )}
        </Stack>
      </Modal>

      <Modal opened={createModalOpen} onClose={handleCloseCreateModal} title={t('newOperator')}
        closeButtonProps={{ 'aria-label': t('closeDialog') }}>
        {createdCredentials ? (
          <Stack>
            <Alert color="green" title={t('operatorCreated')}>
              {t('shareNewCredentials')}
            </Alert>
            <Group>
              <Text size="sm">{t('createdLogin', { login: createdCredentials.login })}</Text>
            </Group>
            <Group>
              <Text size="sm">{t('createdPassword', { password: createdCredentials.password })}</Text>
              <CopyButton value={createdCredentials.password}>
                {({ copied, copy }) => (
                  <Tooltip label={copied ? t('copied') : t('copy')}>
                    <ActionIcon onClick={copy} color={copied ? 'teal' : 'blue'} aria-label={copied ? t('copied') : t('copyPassword')}>
                      {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
                    </ActionIcon>
                  </Tooltip>
                )}
              </CopyButton>
            </Group>
            <Button onClick={handleCloseCreateModal}>{t('done')}</Button>
          </Stack>
        ) : (
          <form onSubmit={createForm.onSubmit((values) => createMutation.mutate(values))}>
            <Stack>
              <TextInput label={t('name')} required {...createForm.getInputProps('name')} />
              <TextInput label={t('login')} required {...createForm.getInputProps('login')} />
              <TextInput label={t('optionalEmail')} {...createForm.getInputProps('email')} />
              <Select
                label={t('role')}
                data={[
                  { value: 'operator', label: t('operator') },
                  { value: 'admin', label: t('admin') },
                ]}
                {...createForm.getInputProps('role')}
              />
              <Button type="submit" loading={createMutation.isPending}>
                {t('create')}
              </Button>
            </Stack>
          </form>
        )}
      </Modal>
    </>
  );
}
