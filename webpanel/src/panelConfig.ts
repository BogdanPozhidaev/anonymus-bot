const colors = ['blue', 'cyan', 'teal', 'green', 'grape', 'violet', 'indigo'];

function primaryColor(value: unknown) {
  return typeof value === 'string' && colors.includes(value) ? value : 'blue';
}

function colorScheme(value: unknown): 'light' | 'dark' | 'auto' {
  return value === 'dark' || value === 'auto' ? value : 'light';
}

function decodeText(value: unknown) {
  if (typeof value !== 'string' || value === '') return '';
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(
      Uint8Array.from(atob(value), char => char.charCodeAt(0)),
    );
  } catch {
    return '';
  }
}

let settings = {
  botUsername: import.meta.env.VITE_BOT_USERNAME ?? '',
  productName: import.meta.env.VITE_PRODUCT_NAME ?? '',
  primaryColor: primaryColor(import.meta.env.VITE_PANEL_PRIMARY_COLOR),
  colorScheme: colorScheme(import.meta.env.VITE_PANEL_COLOR_SCHEME),
  loginSubtitle: {
    ru: import.meta.env.VITE_PANEL_LOGIN_SUBTITLE_RU ?? '',
    en: import.meta.env.VITE_PANEL_LOGIN_SUBTITLE_EN ?? '',
  },
};

export async function loadPanelConfig() {
  try {
    const response = await fetch('/panel-config.json', { cache: 'no-store', signal: AbortSignal.timeout(3000) });
    if (!response.ok) return;
    const config = await response.json();
    if (!config || typeof config !== 'object' || Array.isArray(config)) return;
    const username = typeof config.botUsername === 'string' ? config.botUsername : '';
    settings = {
      botUsername: /^[a-zA-Z0-9_]{5,32}$/.test(username) ? username : '',
      productName: decodeText(config.productNameBase64),
      primaryColor: primaryColor(config.primaryColor),
      colorScheme: colorScheme(config.colorScheme),
      loginSubtitle: {
        ru: decodeText(config.loginSubtitleRuBase64),
        en: decodeText(config.loginSubtitleEnBase64),
      },
    };
  } catch {
    // Vite development can use its environment values without the container endpoint.
  }
}

export function panelConfig() {
  return settings;
}
