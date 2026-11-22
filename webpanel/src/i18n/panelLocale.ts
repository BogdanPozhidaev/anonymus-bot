import { create } from 'zustand';
import { panelConfig } from '../panelConfig';
import type { SessionStatus } from '../types/api';
import { messages, type MessageKey } from './messages';

type Language = 'ru' | 'en';
const storageKey = 'anonymus.panel-language';

function initialLanguage(): Language {
  try {
    return window.localStorage.getItem(storageKey) === 'en' ? 'en' : 'ru';
  } catch {
    return 'ru';
  }
}

export const usePanelLanguage = create<{
  language: Language;
  setLanguage: (language: Language) => void;
}>(set => ({
  language: initialLanguage(),
  setLanguage: language => {
    set({ language });
    try {
      window.localStorage.setItem(storageKey, language);
    } catch {
      // Keep the selection for this page when storage is unavailable.
    }
  },
}));

export const sessionStatusMessages: Record<SessionStatus, MessageKey> = {
  active: 'sessionActive',
  paused: 'sessionPaused',
  pending_close: 'sessionPendingClose',
  closed: 'sessionClosed',
};

export type Translate = (key: MessageKey, values?: Record<string, string | number>) => string;

export function usePanelLocale() {
  const language = usePanelLanguage(state => state.language);
  const t: Translate = (key, values) => messages[key][language === 'ru' ? 0 : 1].replace(
    /\{(\w+)\}/g,
    (placeholder, name) => String(values?.[name] ?? placeholder),
  );
  const locale = language === 'ru' ? 'ru-RU' : 'en-US';
  const config = panelConfig();
  return {
    t, language, locale,
    productName: config.productName || t('productName'),
    loginSubtitle: config.loginSubtitle[language],
  };
}
