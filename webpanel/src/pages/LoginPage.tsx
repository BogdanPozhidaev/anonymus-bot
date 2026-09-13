import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { TextInput, PasswordInput, Button, Paper, Title, Stack, Text, Image, Alert } from '@mantine/core';
import { useForm } from '@mantine/form';
import { notifications } from '@mantine/notifications';
import QRCode from 'qrcode';

import { login, verifyTotp } from '../api/auth';
import { useAuthStore } from '../stores/authStore';

type Step = 'credentials' | 'totp' | 'totp_setup';

export function LoginPage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((s) => s.setSession);

  const [step, setStep] = useState<Step>('credentials');
  const [pendingToken, setPendingToken] = useState('');
  const [totpSecret, setTotpSecret] = useState('');
  const [qrDataUrl, setQrDataUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const credentialsForm = useForm({
    initialValues: { login: '', password: '' },
  });

  const totpForm = useForm({
    initialValues: { code: '' },
  });

  const handleCredentialsSubmit = async (values: { login: string; password: string }) => {
    setLoading(true);
    setError('');

    try {
      const response = await login(values.login, values.password);

      if (response.status === 'totp_setup_required') {
        setPendingToken(response.pending_token!);
        setTotpSecret(response.totp_secret!);

        const dataUrl = await QRCode.toDataURL(response.totp_qr_url!, {
          width: 200,
          margin: 1,
        });
        setQrDataUrl(dataUrl);

        setStep('totp_setup');
      } else if (response.status === 'totp_required') {
        setPendingToken(response.pending_token!);
        setStep('totp');
      }
    } catch (err) {
      setError('Неверный логин или пароль');
    } finally {
      setLoading(false);
    }
  };

  const handleTotpSubmit = async (values: { code: string }) => {
    setLoading(true);
    setError('');

    try {
      const response = await verifyTotp(pendingToken, values.code);
      setSession(response.session_token, response.operator_id, response.role);
      notifications.show({ message: 'Вход выполнен', color: 'green' });
      navigate('/sessions');
    } catch (err) {
      setError('Неверный код подтверждения');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Paper radius="md" p="xl" withBorder maw={420} mx="auto" mt={100}>
      <Title order={2} ta="center" mb="md">
        Панель Посредника
      </Title>

      {error && (
        <Alert color="red" mb="md">
          {error}
        </Alert>
      )}

      {step === 'credentials' && (
        <form onSubmit={credentialsForm.onSubmit(handleCredentialsSubmit)}>
          <Stack>
            <TextInput
              label="Логин"
              required
              {...credentialsForm.getInputProps('login')}
            />
            <PasswordInput
              label="Пароль"
              required
              {...credentialsForm.getInputProps('password')}
            />
            <Button type="submit" loading={loading} fullWidth>
              Войти
            </Button>
          </Stack>
        </form>
      )}

      {step === 'totp_setup' && (
        <Stack>
          <Text size="sm">
            Отсканируйте QR-код в приложении-аутентификаторе (Google Authenticator, Authy и т.п.)
          </Text>
          {qrDataUrl && <Image src={qrDataUrl} w={200} h={200} mx="auto" />}
          <Text size="xs" c="dimmed" ta="center">
            Или введите секрет вручную: {totpSecret}
          </Text>
          <form onSubmit={totpForm.onSubmit(handleTotpSubmit)}>
            <Stack>
              <TextInput
                label="Код из приложения"
                required
                {...totpForm.getInputProps('code')}
              />
              <Button type="submit" loading={loading} fullWidth>
                Подтвердить
              </Button>
            </Stack>
          </form>
        </Stack>
      )}

      {step === 'totp' && (
        <form onSubmit={totpForm.onSubmit(handleTotpSubmit)}>
          <Stack>
            <Text size="sm">Введите код из приложения-аутентификатора</Text>
            <TextInput
              label="Код"
              required
              {...totpForm.getInputProps('code')}
            />
            <Button type="submit" loading={loading} fullWidth>
              Войти
            </Button>
          </Stack>
        </form>
      )}
    </Paper>
  );
}