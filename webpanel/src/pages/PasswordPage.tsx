import { usePanelLocale } from '../i18n/panelLocale';
import type { MessageKey } from '../i18n/messages';
import { useState, type FormEvent } from 'react';
import { Alert, Button, PasswordInput, Stack, Title } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { useNavigate } from 'react-router-dom';
import { changePassword } from '../api/auth';
import { useAuthStore } from '../stores/authStore';

export function PasswordPage() {
  const { t } = usePanelLocale();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const logout = useAuthStore(state => state.logout);
  const passwordChangeRequired = useAuthStore(state => state.passwordChangeRequired);
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [error, setError] = useState<MessageKey | null>(null);
  const [saving, setSaving] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    if (next !== confirmation) {
      setError('passwordMismatch');
      return;
    }
    setError(null);
    setSaving(true);
    try {
      await changePassword(current, next);
      queryClient.clear();
      logout();
      notifications.show({ message: t('passwordChanged'), color: 'green' });
      navigate('/login', { replace: true });
    } catch (err) {
      const status = isAxiosError(err) ? err.response?.status : undefined;
      if (status === 400) setError('passwordInvalidNew');
      else if (status === 403) setError('passwordIncorrect');
      else if (status === 429) setError('attemptsLimited');
      else if (status === 409) setError('accountChanged');
      else setError('passwordChangeFailed');
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={submit}>
      <Stack maw={440}>
        <Title order={2}>{t('passwordChange')}</Title>
        {passwordChangeRequired && <Alert>{t('mandatoryPasswordChange')}</Alert>}
        {error && <Alert color="red" role="alert">{t(error)}</Alert>}
        <PasswordInput label={passwordChangeRequired ? t('temporaryPassword') : t('currentPassword')} autoComplete="current-password" required disabled={saving}
          visibilityToggleButtonProps={{ 'aria-label': t('togglePasswordVisibility') }}
          value={current} onChange={event => setCurrent(event.currentTarget.value)} />
        <PasswordInput label={t('newPassword')} autoComplete="new-password" required disabled={saving}
          visibilityToggleButtonProps={{ 'aria-label': t('togglePasswordVisibility') }}
          description={t('passwordRequirements')}
          value={next} onChange={event => setNext(event.currentTarget.value)} />
        <PasswordInput label={t('repeatNewPassword')} autoComplete="new-password" required disabled={saving}
          visibilityToggleButtonProps={{ 'aria-label': t('togglePasswordVisibility') }}
          value={confirmation} onChange={event => setConfirmation(event.currentTarget.value)} />
        <Button type="submit" loading={saving}>{t('updatePassword')}</Button>
      </Stack>
    </form>
  );
}
