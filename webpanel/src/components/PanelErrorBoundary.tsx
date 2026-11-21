import { Component, type ReactNode } from 'react';
import { Alert, Button, Stack } from '@mantine/core';

export class PanelErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <Alert color="red" title="Не удалось открыть панель" m="md">
        <Stack align="start">
          Обновите страницу, чтобы повторить попытку.
          <Button onClick={() => window.location.reload()}>Обновить страницу</Button>
        </Stack>
      </Alert>
    );
  }
}
