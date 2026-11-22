import { NativeSelect } from '@mantine/core';
import { usePanelLanguage, usePanelLocale } from '../i18n/panelLocale';

export function LanguageSelector() {
  const { t, language } = usePanelLocale();
  const setLanguage = usePanelLanguage(state => state.setLanguage);

  return (
    <NativeSelect
      label={t('language')}
      size="xs"
      value={language}
      onChange={event => setLanguage(event.currentTarget.value === 'en' ? 'en' : 'ru')}
      data={[{ value: 'ru', label: 'Русский' }, { value: 'en', label: 'English' }]}
    />
  );
}
