import { usePanelLocale } from '../i18n/panelLocale';
import { Button, Group } from '@mantine/core';

export function ListPager({ offset, size, hasMore, loading, onChange }: {
  offset: number; size: number; hasMore: boolean; loading: boolean; onChange: (offset: number) => void;
}) {
  const { t } = usePanelLocale();
  if (!offset && !hasMore) return null;
  return (
    <Group justify="end" my="sm">
      <Button variant="default" disabled={loading || offset === 0} onClick={() => onChange(offset - size)}>{t('back')}</Button>
      <Button variant="default" disabled={loading || !hasMore} onClick={() => onChange(offset + size)}>{t('next')}</Button>
    </Group>
  );
}
