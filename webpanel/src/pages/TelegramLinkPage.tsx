import { usePanelLocale } from '../i18n/panelLocale';
import type { MessageKey } from '../i18n/messages';
import { useState, type FormEvent } from 'react';
import { Alert, Button, Code, CopyButton, Group, Stack, Text, TextInput, Title } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { apiClient } from '../api/client';
import { useAuthStore } from '../stores/authStore';

const path = '/account/telegram-link';

export function TelegramLinkPage() {
  const { t } = usePanelLocale();
  const operatorId = useAuthStore(state => state.operatorId);
  const status = useQuery({
    queryKey: ['telegram-link', operatorId],
    queryFn: async () => (await apiClient.get<{ linked: boolean }>(path)).data,
  });
  const [code, setCode] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<MessageKey | null>(null);
  const [saved, setSaved] = useState(false);
  const command = `/link ${code}`;

  async function issue() {
    setPending(true);
    setError(null);
    setSaved(false);
    setCode('');
    setConfirmation('');
    try {
      setCode((await apiClient.post<{ code: string }>(path)).data.code);
    } catch {
      setError('telegramIssueFailed');
    } finally {
      setPending(false);
    }
  }

  async function confirm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || !confirmation.trim()) return;
    setPending(true);
    setError(null);
    try {
      await apiClient.post(`${path}/confirm`, { code: confirmation.trim() });
      setCode('');
      setConfirmation('');
      setSaved(true);
      await status.refetch();
    } catch (err) {
      const httpStatus = isAxiosError(err) ? err.response?.status : undefined;
      if (httpStatus === 409) setError('telegramAlreadyLinked');
      else if (httpStatus === 404) setError('telegramCodeUnavailable');
      else setError('telegramConfirmFailed');
    } finally {
      setPending(false);
    }
  }

  return (
    <Stack maw={540}>
      <Title order={2}>{t('telegramAccount')}</Title>
      {status.isPending && <Text>{t('telegramChecking')}</Text>}
      {status.isError && <Alert color="red">{t('telegramCheckFailed')}</Alert>}
      {status.data && <Text>{status.data.linked ? t('telegramLinked') : t('telegramNotLinked')}</Text>}
      {saved && <Alert color="green" role="status">{t('telegramConfirmed')}</Alert>}
      {error && <Alert color="red" role="alert">{t(error)}</Alert>}
      <Button onClick={issue} disabled={pending || !status.data}>
        {code ? t('telegramNewCode') : status.data?.linked ? t('telegramRelink') : t('telegramStart')}
      </Button>
      {code && (
        <form onSubmit={confirm}>
          <Stack>
            <Text>{t('telegramInstructions')}</Text>
            <Group>
              <Code>{command}</Code>
              <CopyButton value={command}>
                {({ copied, copy }) => <Button variant="default" onClick={copy}>{copied ? t('copied') : t('copyCommand')}</Button>}
              </CopyButton>
            </Group>
            <TextInput label={t('telegramReplyCode')} required maxLength={64} autoComplete="off" spellCheck={false}
              disabled={pending} value={confirmation} onChange={event => setConfirmation(event.currentTarget.value)} />
            <Button type="submit" loading={pending}>{t('telegramConfirm')}</Button>
          </Stack>
        </form>
      )}
    </Stack>
  );
}
