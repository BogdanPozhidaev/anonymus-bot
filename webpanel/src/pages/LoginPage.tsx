import { usePanelLocale } from '../i18n/panelLocale';
import type { MessageKey } from '../i18n/messages';
import { LanguageSelector } from '../components/LanguageSelector';
import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { TextInput, PasswordInput, Button, Paper, Title, Stack, Text, Image, Alert } from '@mantine/core';
import { useForm } from '@mantine/form';
import { notifications } from '@mantine/notifications';
import QRCode from 'qrcode';
import { isAxiosError } from 'axios';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { completeOIDC, login, oidcEnabled, recoverTotp, startOIDC, verifyTotp } from '../api/auth';
import type { LoginResponse } from '../types/api';
import { useAuthStore } from '../stores/authStore';

type Step = 'credentials' | 'totp' | 'totp_setup' | 'recovery';

type LoginError = { key: MessageKey; seconds?: number };

function authErrorMessage(error: unknown, fallback: MessageKey): LoginError {
  if (!isAxiosError(error)) return { key: fallback };
  if (error.response?.status === 429) {
    const seconds = Number(error.response.headers['retry-after']);
    return Number.isFinite(seconds) && seconds > 0
      ? { key: 'loginRateLimitSeconds', seconds }
      : { key: 'loginRateLimit' };
  }
  if (error.response?.status === 503) return { key: 'loginUnavailable' };
  return { key: fallback };
}

export function LoginPage() {
  const { t, productName, loginSubtitle } = usePanelLocale();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const setSession = useAuthStore((s) => s.setSession);
  const [oidcCallback] = useState(() => new URLSearchParams(window.location.search).get('oidc'));
  const callbackHandled = useRef(false);
  const { data: allowOIDC } = useQuery({ queryKey: ['oidc-enabled'], queryFn: oidcEnabled });

  const [step, setStep] = useState<Step>('credentials');
  const [pendingToken, setPendingToken] = useState('');
  const [totpSecret, setTotpSecret] = useState('');
  const [qrDataUrl, setQrDataUrl] = useState('');
  const [loading, setLoading] = useState(oidcCallback === 'complete');
  const [error, setError] = useState<LoginError | null>(oidcCallback === 'failed' ? { key: 'oidcFailed' } : null);
  const [recoveryCode, setRecoveryCode] = useState('');

  useEffect(() => {
    if (!oidcCallback || callbackHandled.current) return;
    callbackHandled.current = true;
    navigate('/login', { replace: true });
    if (oidcCallback === 'complete') {
      void completeOIDC().then(response => {
        if (response.status !== 'totp_required' || !response.pending_token) throw new Error('Incomplete sign-in');
        setPendingToken(response.pending_token);
        setStep('totp');
      }).catch(error => setError(authErrorMessage(error, 'oidcFailed'))).finally(() => setLoading(false));
    }
  }, [navigate, oidcCallback]);

  async function startSingleSignOn() {
    setLoading(true);
    setError(null);
    try {
      window.location.assign(await startOIDC());
    } catch (err) {
      setError(authErrorMessage(err, 'oidcFailed'));
      setLoading(false);
    }
  }

  const credentialsForm = useForm({
    initialValues: { login: '', password: '' },
  });

  const totpForm = useForm({
    initialValues: { code: '' },
  });

  async function showEnrollment(response: LoginResponse) {
    if (!response.pending_token || !response.totp_secret || !response.totp_qr_url) {
      throw new Error('Incomplete enrollment data');
    }
    const dataUrl = await QRCode.toDataURL(response.totp_qr_url, { width: 200, margin: 1 }).catch(() => '');
    setPendingToken(response.pending_token);
    setTotpSecret(response.totp_secret);
    setQrDataUrl(dataUrl);
    totpForm.reset();
    setStep('totp_setup');
  }

  const handleCredentialsSubmit = async (values: { login: string; password: string }) => {
    setLoading(true);
    setError(null);

    try {
      const response = await login(values.login, values.password);

      if (response.status === 'totp_setup_required') {
        await showEnrollment(response);
      } else if (response.status === 'totp_required') {
        setPendingToken(response.pending_token!);
        setStep('totp');
      }
      credentialsForm.setFieldValue('password', '');
    } catch (err) {
      setError(authErrorMessage(err, 'invalidCredentials'));
    } finally {
      setLoading(false);
    }
  };

  const handleRecovery = async () => {
    setLoading(true);
    setError(null);
    try {
      await showEnrollment(await recoverTotp(pendingToken, recoveryCode));
    } catch (err) {
      setPendingToken('');
      setStep('credentials');
      setError(authErrorMessage(err, 'recoveryRejected'));
    } finally {
      setRecoveryCode('');
      setLoading(false);
    }
  };

  const handleTotpSubmit = async (values: { code: string }) => {
    setLoading(true);
    setError(null);

    try {
      const response = await verifyTotp(pendingToken, values.code);
      queryClient.clear();
      setSession(response.session_token, response.operator_id, response.role, response.password_change_required);
      if (response.password_change_required) {
        navigate('/account/password', { replace: true });
        return;
      }
      notifications.show({ message: t('signedIn'), color: 'green' });
      navigate('/dashboard', { replace: true });
    } catch (err) {
      setError(authErrorMessage(err, 'invalidTotp'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Paper radius="md" p="xl" withBorder maw={420} mx="auto" mt={100}>
      <Title order={2} ta="center" mb="md">
        {productName}
      </Title>
      {loginSubtitle && <Text ta="center" mb="md" style={{ whiteSpace: 'pre-wrap' }}>{loginSubtitle}</Text>}
      <Stack align="flex-end" mb="md"><LanguageSelector /></Stack>

      {error && (
        <Alert color="red" mb="md">
          {t(error.key, error.seconds === undefined ? undefined : { seconds: error.seconds })}
        </Alert>
      )}

      {step === 'credentials' && (
        <form onSubmit={credentialsForm.onSubmit(handleCredentialsSubmit)}>
          <Stack>
            <TextInput
              label={t('login')}
              required
              {...credentialsForm.getInputProps('login')}
            />
            <PasswordInput
              label={t('password')}
              visibilityToggleButtonProps={{ 'aria-label': t('togglePasswordVisibility') }}
              required
              {...credentialsForm.getInputProps('password')}
            />
            <Button type="submit" loading={loading} fullWidth>
              {t('signIn')}
            </Button>
          </Stack>
        </form>
      )}

      {step === 'credentials' && allowOIDC && (
        <Button variant="default" fullWidth mt="sm" disabled={loading} onClick={() => void startSingleSignOn()}>
          {t('oidcSignIn')}
        </Button>
      )}

      {step === 'recovery' && (
        <form onSubmit={event => { event.preventDefault(); if (!loading) void handleRecovery(); }}>
          <Stack>
            <Text size="sm">{t('recoveryInstructions')}</Text>
            <TextInput label={t('recoveryCode')} required autoComplete="off" maxLength={64} value={recoveryCode}
              onChange={event => setRecoveryCode(event.currentTarget.value)} />
            <Button type="submit" loading={loading}>{t('recoverTotp')}</Button>
            <Button type="button" variant="subtle" disabled={loading} onClick={() => { setRecoveryCode(''); setStep('totp'); }}>
              {t('backToTotp')}
            </Button>
          </Stack>
        </form>
      )}

      {step === 'totp_setup' && (
        <Stack>
          <Text size="sm">
            {t('scanQr')}
          </Text>
          {qrDataUrl && <Image src={qrDataUrl} alt={t('qrCode')} w={200} h={200} mx="auto" />}
          <Text size="xs" c="dimmed" ta="center">
            {t('manualSecret', { secret: totpSecret })}
          </Text>
          <form onSubmit={totpForm.onSubmit(handleTotpSubmit)}>
            <Stack>
              <TextInput
                label={t('authenticatorCode')}
                required
                {...totpForm.getInputProps('code')}
              />
              <Button type="submit" loading={loading} fullWidth>
                {t('confirm')}
              </Button>
              <Button type="button" variant="subtle" disabled={loading} onClick={() => {
                setStep('credentials');
                setPendingToken('');
                setTotpSecret('');
                setQrDataUrl('');
                totpForm.reset();
                setError(null);
              }}>
                {t('restartLogin')}
              </Button>
            </Stack>
          </form>
        </Stack>
      )}

      {step === 'totp' && (
        <form onSubmit={totpForm.onSubmit(handleTotpSubmit)}>
          <Stack>
            <Text size="sm">{t('enterAuthenticatorCode')}</Text>
            <TextInput
              label={t('code')}
              required
              {...totpForm.getInputProps('code')}
            />
            <Button type="submit" loading={loading} fullWidth>
              {t('signIn')}
            </Button>
            <Button type="button" variant="subtle" disabled={loading} onClick={() => { setError(null); setStep('recovery'); }}>
              {t('useRecoveryCode')}
            </Button>
          </Stack>
        </form>
      )}
    </Paper>
  );
}
