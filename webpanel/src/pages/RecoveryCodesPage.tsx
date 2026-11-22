import { usePanelLocale } from '../i18n/panelLocale';
import type { MessageKey } from '../i18n/messages';
import { useState, type FormEvent } from 'react';
import { Alert, Button, Code, CopyButton, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core';
import { useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { useNavigate } from 'react-router-dom';
import { issueRecoveryCodes } from '../api/auth';
import { useAuthStore } from '../stores/authStore';

export function RecoveryCodesPage() {
  const { t } = usePanelLocale();
  const navigate = useNavigate();
  const queries = useQueryClient();
  const logout = useAuthStore(state => state.logout);
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [codes, setCodes] = useState<string[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<MessageKey | null>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;
    setPending(true);
    setError(null);
    try {
      setCodes(await issueRecoveryCodes(password, code));
      queries.clear();
      setPassword('');
      setCode('');
    } catch (err) {
      const status = isAxiosError(err) ? err.response?.status : undefined;
      if (status === 403) setError('credentialsOrTotpInvalid');
      else if (status === 429) setError('attemptsLimited');
      else setError('issueRecoveryFailed');
    } finally {
      setPending(false);
    }
  }

  if (codes.length) {
    return (
      <Stack maw={480}>
        <Title order={2}>{t('recoveryCodes')}</Title>
        <Alert>{t('storeRecoveryCodes')}</Alert>
        <Code block>{codes.join('\n')}</Code>
        <CopyButton value={codes.join('\n')}>
          {({ copied, copy }) => <Button variant="default" onClick={copy}>{copied ? t('copied') : t('copyCodes')}</Button>}
        </CopyButton>
        <Button onClick={() => { logout(); navigate('/login', { replace: true }); }}>{t('doneSignInAgain')}</Button>
      </Stack>
    );
  }

  return (
    <form onSubmit={submit}>
      <Stack maw={480}>
        <Title order={2}>{t('recoveryCodes')}</Title>
        <Text>{t('reissueRecoveryEffect')}</Text>
        {error && <Alert color="red" role="alert">{t(error)}</Alert>}
        <PasswordInput label={t('currentPassword')} autoComplete="current-password" required disabled={pending} value={password}
          visibilityToggleButtonProps={{ 'aria-label': t('togglePasswordVisibility') }}
          onChange={event => setPassword(event.currentTarget.value)} />
        <TextInput label={t('totpCode')} autoComplete="one-time-code" required disabled={pending} value={code} maxLength={16}
          onChange={event => setCode(event.currentTarget.value)} />
        <Button type="submit" loading={pending}>{t('issueRecoveryCodes')}</Button>
      </Stack>
    </form>
  );
}
