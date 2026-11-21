const defaultName = 'Панель Посредника';

let settings = {
  botUsername: import.meta.env.VITE_BOT_USERNAME ?? '',
  productName: import.meta.env.VITE_PRODUCT_NAME ?? defaultName,
};

export async function loadPanelConfig() {
  try {
    const response = await fetch('/panel-config.json', { cache: 'no-store', signal: AbortSignal.timeout(3000) });
    if (!response.ok) return;
    const config: { botUsername?: string; productNameBase64?: string } = await response.json();
    const productName = config.productNameBase64
      ? new TextDecoder().decode(Uint8Array.from(atob(config.productNameBase64), char => char.charCodeAt(0)))
      : defaultName;
    const username = config.botUsername ?? '';
    settings = {
      botUsername: /^[a-zA-Z0-9_]{5,32}$/.test(username) ? username : '',
      productName,
    };
  } catch {
    // Vite development can use its environment values without the container endpoint.
  }
}

export function panelConfig() {
  return settings;
}
