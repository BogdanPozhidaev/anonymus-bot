import { usePanelLocale } from '../i18n/panelLocale';
import { Component, type ReactNode } from 'react';
import { Alert, Button, Stack } from '@mantine/core';

export class PanelErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return <PanelError />;
  }
}

function PanelError() {
  const { t } = usePanelLocale();
  return (
    <Alert color="red" title={t('panelError')} m="md">
      <Stack align="start">
        {t('retryReload')}
        <Button onClick={() => window.location.reload()}>{t('reload')}</Button>
      </Stack>
    </Alert>
  );
}
