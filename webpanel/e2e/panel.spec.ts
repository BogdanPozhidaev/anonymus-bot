import { expect, test as base } from '@playwright/test';
import type { Page } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import type { Message, OperatorListItem, OperatorRole, SessionListItem } from '../src/types/api.js';

const fixture = {
  login: 'fixture-operator',
  password: 'fixture-only-password',
  totp: '654321',
  pendingToken: 'fixture-pending',
  sessionToken: 'fixture-session',
  timestamp: '2026-01-01T12:00:00Z',
};

const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5a0AAAAASUVORK5CYII=', 'base64');

function silentWave() {
  const sampleRate = 8000;
  const dataSize = sampleRate / 2;
  const wav = Buffer.alloc(44 + dataSize, 128);
  wav.write('RIFF', 0);
  wav.writeUInt32LE(36 + dataSize, 4);
  wav.write('WAVEfmt ', 8);
  wav.writeUInt32LE(16, 16);
  wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22);
  wav.writeUInt32LE(sampleRate, 24);
  wav.writeUInt32LE(sampleRate, 28);
  wav.writeUInt16LE(1, 32);
  wav.writeUInt16LE(8, 34);
  wav.write('data', 36);
  wav.writeUInt32LE(dataSize, 40);
  return wav;
}

function message(id: number, kind: Message['content_type'], content: string | null): Message {
  return {
    id, session_id: 42, sender_user_id: 12, sender_role: 'client', content_type: kind, content,
    file_id: kind === 'text' ? null : `fixture-media-${id}`, sent_by_operator: false, impersonated_role: null,
    delivered: true, delivery_status: 'delivered', created_at: fixture.timestamp,
  };
}

const test = base.extend<{
  sessionFixture: SessionListItem;
  operatorRole: OperatorRole;
  dashboardState: 'normal' | 'empty' | 'request-error';
  transcriptTooLarge: boolean;
  oidcAvailable: boolean;
}>({
  operatorRole: ['operator', { option: true }],
  dashboardState: ['normal', { option: true }],
  transcriptTooLarge: [false, { option: true }],
  oidcAvailable: [false, { option: true }],
  sessionFixture: [async ({ page, baseURL, operatorRole, dashboardState, transcriptTooLarge, oidcAvailable }, useFixture) => {
    const session: SessionListItem = {
      id: 42, title: 'Fixture session', status: 'active', owner_operator_id: 7,
      client_user_id: null, executor_user_id: null, created_at: fixture.timestamp,
    };
    const messages = [message(1, 'text', 'Fixture history'), message(2, 'photo', null), message(3, 'voice', null)];
    const operators: OperatorListItem[] = [
      { id: 7, name: 'Fixture admin', login: 'fixture-admin', role: 'admin', status: 'active', email: null, created_at: fixture.timestamp, last_login_at: null },
      { id: 8, name: 'Fixture target', login: 'fixture-target', role: 'admin', status: 'active', email: null, created_at: fixture.timestamp, last_login_at: null },
    ];
    const unexpected: string[] = [];
    let oidcCompleted = false;

    await page.route('**/*', async route => {
      const request = route.request();
      const { origin, pathname, searchParams } = new URL(request.url());
      if (origin !== baseURL) {
        unexpected.push('unexpected external request');
        return route.abort();
      }
      if (pathname === '/panel-config.json') {
        return route.fulfill({ json: { botUsername: 'fixture_bot' } });
      }
      if (oidcAvailable && pathname === '/provider-fixture') {
        return route.fulfill({ status: 303, headers: { location: '/login?oidc=complete' } });
      }
      if (!pathname.startsWith('/api/')) {
        return route.continue();
      }

      const key = `${request.method()} ${pathname}`;
      const body = request.postData() ? request.postDataJSON() : undefined;
      if (key === 'GET /api/auth/oidc') {
        return route.fulfill({ json: { enabled: oidcAvailable } });
      }
      if (oidcAvailable && key === 'POST /api/auth/oidc/start') {
        return route.fulfill({ json: { authorization_url: `${baseURL}/provider-fixture` } });
      }
      if (oidcAvailable && key === 'POST /api/auth/oidc/complete' && !oidcCompleted) {
        oidcCompleted = true;
        return route.fulfill({ json: { status: 'totp_required', pending_token: fixture.pendingToken } });
      }
      if (key === 'POST /api/auth/login') {
        if (body?.login !== fixture.login || body?.password !== fixture.password) return route.fulfill({ status: 401 });
        return route.fulfill({ json: { status: 'totp_required', pending_token: fixture.pendingToken } });
      }
      if (key === 'POST /api/auth/totp/verify') {
        if (body?.pending_token !== fixture.pendingToken || body?.code !== fixture.totp) return route.fulfill({ status: 401 });
        return route.fulfill({ json: { session_token: fixture.sessionToken, operator_id: 7, role: operatorRole } });
      }
      if ((await request.headerValue('authorization')) !== `Bearer ${fixture.sessionToken}`) {
        unexpected.push(`missing authorization for ${key}`);
        return route.fulfill({ status: 401 });
      }

      switch (key) {
        case 'POST /api/auth/logout':
          return route.fulfill({ json: { status: 'ok' } });
        case 'GET /api/sessions':
          return route.fulfill({ json: { sessions: dashboardState === 'empty' ? [] : [session], has_more: false } });
        case 'GET /api/incoming-requests': {
          if (searchParams.get('status') !== 'new') unexpected.push('dashboard requested the wrong queue');
          if (dashboardState === 'request-error') return route.fulfill({ status: 503 });
          const requests = dashboardState === 'empty' ? [] : [{ id: 11, first_name: 'Fixture request', created_at: fixture.timestamp }];
          return route.fulfill({ json: { requests, has_more: false } });
        }
        case 'GET /api/operators':
          if (operatorRole !== 'admin') unexpected.push('operator requested administrative data');
          return route.fulfill({ json: { operators, has_more: false } });
        case 'PATCH /api/operators/7':
        case 'PATCH /api/operators/8': {
          const target = operators.find(op => pathname.endsWith(`/${op.id}`));
          if (operatorRole !== 'admin' || !target || body?.role !== 'operator' || Object.keys(body).length !== 1) {
            unexpected.push('invalid role change request');
            return route.fulfill({ status: 400 });
          }
          if (target.id === 7 && operators[1].role !== 'admin') {
            return route.fulfill({ status: 409, json: { error: 'cannot remove the last active administrator' } });
          }
          target.role = body.role;
          return route.fulfill({ json: { status: 'ok' } });
        }
        case 'GET /api/sessions/42':
          return route.fulfill({ json: session });
        case 'GET /api/sessions/42/messages':
          return route.fulfill({ json: { messages, has_more: false } });
        case 'GET /api/sessions/42/transcript':
          if (transcriptTooLarge) return route.fulfill({ status: 422, json: { error: 'transcript exceeds export limits' } });
          if (searchParams.get('format') === 'csv') {
            return route.fulfill({ contentType: 'text/csv', body: 'content\nFixture transcript\n' });
          }
          if (searchParams.get('format') === 'pdf') {
            return route.fulfill({ contentType: 'application/pdf', body: '%PDF-1.4\nfixture' });
          }
          unexpected.push('invalid transcript format');
          return route.fulfill({ status: 400 });
        case 'GET /api/messages/2/media':
          return route.fulfill({ contentType: 'image/png', body: image });
        case 'GET /api/messages/3/media':
          return route.fulfill({ contentType: 'audio/wav', body: silentWave() });
        case 'POST /api/sessions/42/send-as':
          if (body?.impersonated_role !== 'executor' || body?.content_type !== 'text' ||
              body?.content !== 'Fixture panel reply' || !session.client_user_id) {
            unexpected.push('invalid send-as request');
            return route.fulfill({ status: 400 });
          }
          messages.push({
            ...message(4, 'text', body.content), sender_role: 'operator', sender_user_id: null,
            sent_by_operator: true, impersonated_role: 'executor', delivered: false, delivery_status: 'queued',
          });
          return route.fulfill({ json: { message_id: 4 } });
        default:
          unexpected.push(key);
          return route.fulfill({ status: 404 });
      }
    });

    await useFixture(session);
    expect(unexpected).toEqual([]);
  }, { auto: true }],
});

const panelLabels = {
  ru: {
    login: 'Логин', password: 'Пароль', signIn: 'Войти', code: 'Код', invalidCode: 'Неверный код подтверждения',
    sessions: 'Сессии', openSession: 'Открыть сессию', delivered: 'доставлено', photo: 'Фото',
    sendAs: 'Отправить от имени', send: 'Отправить', message: 'Сообщение', client: 'Клиент',
    executor: 'Исполнитель', queued: 'ожидает доставки', language: 'Язык', active: 'Активна',
  },
  en: {
    login: 'Username', password: 'Password', signIn: 'Sign in', code: 'Code', invalidCode: 'Incorrect verification code',
    sessions: 'Sessions', openSession: 'Open session', delivered: 'delivered', photo: 'Photo',
    sendAs: 'Send as', send: 'Send', message: 'Message', client: 'Client',
    executor: 'Service provider', queued: 'awaiting delivery', language: 'Language', active: 'Active',
  },
};

type Labels = typeof panelLabels.ru;

async function enterCredentials(page: Page, labels = panelLabels.ru) {
  await page.getByLabel(labels.login).fill(fixture.login);
  await page.getByLabel(labels.password).and(page.locator('input[type=password]')).fill(fixture.password);
  await page.getByRole('button', { name: labels.signIn, exact: true }).click();
  await expect(page.getByRole('textbox', { name: labels.code, exact: true })).toBeVisible();
}

async function enterTOTP(page: Page, labels = panelLabels.ru, code = fixture.totp) {
  await page.getByRole('textbox', { name: labels.code, exact: true }).fill(code);
  await page.getByRole('button', { name: labels.signIn, exact: true }).click();
}

async function openSession(page: Page, labels: Labels) {
  await page.getByRole('row').filter({ hasText: 'Fixture session' }).getByRole('button', { name: labels.openSession }).click();
  await expect(page.getByRole('heading', { name: 'Fixture session' })).toBeVisible();
}

test('local login hides SSO when the backend disables it', async ({ page }) => {
  const status = page.waitForResponse(response => new URL(response.url()).pathname === '/api/auth/oidc');
  await page.goto('/login');
  await status;
  await expect(page.getByRole('button', { name: 'Войти через SSO' })).toHaveCount(0);
  await enterCredentials(page);
});

test.describe('optional SSO', () => {
  test.use({ oidcAvailable: true });

  test('returns from the provider and requires a second factor', async ({ page }) => {
    await page.goto('/login');
    await page.getByRole('button', { name: 'Войти через SSO' }).click();
    await expect(page.getByRole('textbox', { name: panelLabels.ru.code, exact: true })).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
    await enterTOTP(page);
    await expect(page).toHaveURL(/\/dashboard$/);
  });

  test('shows a failed callback and leaves password sign-in available', async ({ page }) => {
    await page.goto('/login?oidc=failed');
    await expect(page.getByText('Вход через SSO не завершён. Попробуйте снова или войдите по паролю.')).toBeVisible();
    await expect(page).toHaveURL(/\/login$/);
    await enterCredentials(page);
    await enterTOTP(page);
    await expect(page).toHaveURL(/\/dashboard$/);
  });
});

for (const [language, labels] of Object.entries(panelLabels)) {
  test(`operator can preview delivered media and reply after the client joins (${language})`, async ({ page, sessionFixture }) => {
    await page.goto('/sessions');
    await expect(page).toHaveURL(/\/login$/);
    await page.getByRole('combobox', { name: panelLabels.ru.language }).selectOption(language);
    await enterCredentials(page, labels);
    await enterTOTP(page, labels, '000000');
    await expect(page.getByText(labels.invalidCode)).toBeVisible();
    await expect(page.getByRole('heading', { name: labels.sessions })).toHaveCount(0);

    await enterTOTP(page, labels);
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.getByText('Fixture request', { exact: false })).toBeVisible();
    await page.getByRole('link', { name: 'Fixture session' }).click();
    await expect(page.getByRole('heading', { name: 'Fixture session' })).toBeVisible();
    await expect(page.getByText(labels.active, { exact: true }).filter({ visible: true })).toBeVisible();
    await expect(page.getByText('Fixture history')).toBeVisible();
    await expect(page.getByText(labels.delivered, { exact: false })).toHaveCount(3);
    await expect.poll(() => page.getByRole('img', { name: labels.photo }).evaluate((img: HTMLImageElement) => img.naturalWidth)).toBeGreaterThan(0);
    await expect.poll(() => page.locator('audio').evaluate((audio: HTMLAudioElement) => audio.duration)).toBe(0.5);

    const sender = page.getByRole('combobox', { name: labels.sendAs });
    const submit = page.getByRole('button', { name: labels.send, exact: true });
    const reply = page.getByRole('textbox', { name: labels.message });
    await reply.fill('Fixture panel reply');
    await sender.click();
    await page.getByRole('option', { name: labels.client }).click();
    await expect(sender).toHaveValue('');
    await page.getByRole('option', { name: labels.executor }).click();
    await expect(sender).toHaveValue('');
    await expect(submit).toBeDisabled();
    await sender.press('Escape');

    sessionFixture.client_user_id = 12;
    await page.getByText(labels.sessions, { exact: true }).click();
    await openSession(page, labels);
    await sender.click();
    await page.getByRole('option', { name: labels.client }).click();
    await expect(sender).toHaveValue('');
    await page.getByRole('option', { name: labels.executor }).click();
    await reply.fill('Fixture panel reply');
    await submit.click();
    await expect(page.getByText('Fixture panel reply')).toBeVisible();
    await expect(page.getByText(labels.queued, { exact: false })).toBeVisible();
  });
}

test('session transcript can be downloaded in both formats', async ({ page }) => {
  await page.goto('/login');
  await enterCredentials(page);
  await enterTOTP(page);
  await expect(page).toHaveURL(/\/dashboard$/);
  await page.getByRole('link', { name: 'Fixture session' }).click();
  await expect(page.getByRole('heading', { name: 'Fixture session' })).toBeVisible();
  for (const [format, body] of [['csv', 'content\nFixture transcript\n'], ['pdf', '%PDF-1.4\nfixture']]) {
    const pending = page.waitForEvent('download');
    await page.getByRole('button', { name: `Скачать ${format.toUpperCase()}` }).click();
    const download = await pending;
    expect(download.suggestedFilename()).toBe(`session-42.${format}`);
    expect(await readFile(await download.path(), 'utf8')).toBe(body);
  }
});

test.describe('oversized transcript', () => {
  test.use({ transcriptTooLarge: true });
  test('shows the export limit when the server rejects a download', async ({ page }) => {
    await page.goto('/login');
    await enterCredentials(page);
    await enterTOTP(page);
    await expect(page).toHaveURL(/\/dashboard$/);
    await page.getByRole('link', { name: 'Fixture session' }).click();
    await expect(page.getByRole('heading', { name: 'Fixture session' })).toBeVisible();
    await page.getByRole('button', { name: 'Скачать PDF' }).click();
    await expect(page.getByText('Переписка превышает лимит экспорта')).toBeVisible();
  });
});

test('runtime branding applies the title, scheme, accent, and localized sign-in text', async ({ page }) => {
  const product = 'North & Co «Панель»';
  const ru = 'Вход сотрудников <North & Co>';
  const en = 'Sign in to North & Co';
  const encode = (text: string) => Buffer.from(text, 'utf8').toString('base64');
  await page.route('**/panel-config.json', route => route.fulfill({ json: {
    productNameBase64: encode(product), primaryColor: 'teal', colorScheme: 'dark',
    loginSubtitleRuBase64: encode(ru), loginSubtitleEnBase64: encode(en),
  } }));
  await page.goto('/login');
  await expect(page).toHaveTitle(product);
  await expect(page.getByRole('heading', { name: product })).toBeVisible();
  await expect(page.getByText(ru, { exact: true })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'dark');
  expect(await page.locator('html').evaluate(element =>
    getComputedStyle(element).getPropertyValue('--mantine-primary-color-6').trim()
  )).toBe(await page.locator('html').evaluate(element =>
    getComputedStyle(element).getPropertyValue('--mantine-color-teal-6').trim()
  ));
  await page.getByRole('combobox', { name: panelLabels.ru.language }).selectOption('en');
  await expect(page.getByText(en, { exact: true })).toBeVisible();
  await expect(page.getByText(ru, { exact: true })).toHaveCount(0);
});

test('invalid optional runtime branding falls back to panel defaults', async ({ page }) => {
  await page.route('**/panel-config.json', route => route.fulfill({ json: {
    productNameBase64: '!', primaryColor: 'unsupported', colorScheme: 'unsupported',
    loginSubtitleRuBase64: Buffer.from([0xff]).toString('base64'),
  } }));
  await page.goto('/login');
  await expect(page).toHaveTitle('Панель Посредника');
  await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'light');
  const colors = await page.locator('html').evaluate(element => {
    const style = getComputedStyle(element);
    return ['--mantine-primary-color-6', '--mantine-color-blue-6'].map(name => style.getPropertyValue(name).trim());
  });
  expect(colors[0]).toBeTruthy();
  expect(colors[0]).toBe(colors[1]);
  await expect(page.getByText('�')).toHaveCount(0);
});

test('automatic color scheme follows the browser', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' });
  await page.route('**/panel-config.json', route => route.fulfill({ json: { colorScheme: 'auto' } }));
  await page.goto('/login');
  await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'dark');
  await page.emulateMedia({ colorScheme: 'light' });
  await expect(page.locator('html')).toHaveAttribute('data-mantine-color-scheme', 'light');
});

test('language persists after reload and translates the current sign-in step', async ({ page }) => {
  await page.goto('/login');
  await expect(page.locator('html')).toHaveAttribute('lang', 'ru');
  await page.getByRole('combobox', { name: 'Язык' }).selectOption('en');
  await expect(page).toHaveTitle('Mediator Panel');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await expect(page.getByRole('combobox', { name: 'Language' })).toHaveValue('en');

  await enterCredentials(page, panelLabels.en);
  await enterTOTP(page, panelLabels.en, '000000');
  await expect(page.getByText(panelLabels.en.invalidCode)).toBeVisible();
  await page.getByRole('combobox', { name: 'Language' }).selectOption('ru');
  await expect(page.getByText(panelLabels.ru.invalidCode)).toBeVisible();
  await enterTOTP(page);
  await expect(page.getByRole('heading', { name: 'Обзор' })).toBeVisible();
  await page.getByRole('combobox', { name: 'Язык' }).selectOption('en');
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
});

test('operator cannot open admin routes within an authenticated session', async ({ page }) => {
  await page.goto('/login');
  await enterCredentials(page);
  await enterTOTP(page);
  await expect(page).toHaveURL(/\/dashboard$/);
  for (const [path, label] of [['/operators', 'Операторы'], ['/audit-log', 'Аудит']]) {
    await expect(page.getByText(label, { exact: true })).toHaveCount(0);
    // Keep the in-memory login while exercising the router's URL guard.
    await page.evaluate(url => {
      window.history.pushState({}, '', url);
      window.dispatchEvent(new PopStateEvent('popstate'));
    }, path);
    await expect(page).toHaveURL(/\/sessions$/);
    await expect(page.getByRole('heading', { name: 'Сессии' })).toBeVisible();
  }
});

test('login and TOTP forms fit a narrow viewport', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 667 });
  await page.goto('/login');
  await enterCredentials(page);
  await expect(page.getByRole('button', { name: 'Войти' })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(375);
  await enterTOTP(page);
  await expect(page.getByRole('heading', { name: 'Обзор' })).toBeInViewport();
  await page.getByRole('button', { name: 'Открыть меню' }).click();
  await page.getByText('Сессии', { exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Сессии' })).toBeInViewport();
  await expect(page.getByRole('button', { name: 'Открыть меню' })).toHaveAttribute('aria-expanded', 'false');
});

test.describe('administrator role editor', () => {
  test.use({ operatorRole: 'admin' });

  test('confirms a role change and explains the last administrator conflict', async ({ page }) => {
    await page.goto('/login');
    await enterCredentials(page);
    await enterTOTP(page);
    await page.getByText('Операторы', { exact: true }).click();
    const target = page.getByRole('row').filter({ hasText: 'Fixture target' });
    const own = page.getByRole('row').filter({ hasText: 'Fixture admin' });

    async function chooseOperator(row: typeof target) {
      await row.getByRole('button', { name: 'Изменить роль' }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('button', { name: 'Подтвердить' })).toBeDisabled();
      await dialog.getByRole('combobox', { name: 'Роль' }).click();
      await page.getByRole('option', { name: 'Оператор', exact: true }).click();
      return dialog;
    }

    let dialog = await chooseOperator(target);
    await dialog.getByRole('button', { name: 'Отмена' }).click();
    await expect(target.getByRole('cell', { name: 'Админ', exact: true })).toBeVisible();
    dialog = await chooseOperator(target);
    await dialog.getByRole('button', { name: 'Подтвердить' }).click();
    await expect(target.getByRole('cell', { name: 'Оператор', exact: true })).toBeVisible();

    dialog = await chooseOperator(own);
    await dialog.getByRole('button', { name: 'Подтвердить' }).click();
    await expect(dialog.getByRole('alert')).toHaveText('В системе должен остаться хотя бы один активный администратор.');
    await expect(own.getByRole('cell', { name: 'Админ', exact: true })).toBeVisible();
  });
});

test.describe('empty dashboard', () => {
  test.use({ dashboardState: 'empty' });
  test('shows independent empty states', async ({ page }) => {
    await page.goto('/login');
    await enterCredentials(page);
    await enterTOTP(page);
    await expect(page.getByText('Сессий пока нет')).toBeVisible();
    await expect(page.getByText('Новых обращений нет')).toBeVisible();
    await page.getByRole('link', { name: 'Все обращения' }).click();
    await expect(page.getByRole('heading', { name: 'Входящие обращения' })).toBeVisible();
  });
});

test('a new sign-in does not show the previous dashboard cache', async ({ page, sessionFixture }) => {
  await page.goto('/login');
  await enterCredentials(page);
  await enterTOTP(page);
  await expect(page.getByRole('link', { name: 'Fixture session' })).toBeVisible();
  await page.getByRole('button', { name: 'Выйти' }).click();
  await expect(page).toHaveURL(/\/login$/);
  sessionFixture.title = 'Fixture next session';

  let releaseResponse: () => void = () => undefined;
  const responseGate = new Promise<void>(resolve => { releaseResponse = resolve; });
  await page.route(url => url.pathname === '/api/sessions' && url.searchParams.get('limit') === '5', async route => {
    await responseGate;
    return route.fallback();
  });
  try {
    await enterCredentials(page);
    await enterTOTP(page);
    await expect(page.getByRole('heading', { name: 'Обзор' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Fixture session' })).toHaveCount(0);
  } finally {
    releaseResponse();
  }
  await expect(page.getByRole('link', { name: 'Fixture next session' })).toBeVisible();
});

test.describe('dashboard with unavailable requests', () => {
  test.use({ dashboardState: 'request-error' });
  test('keeps the session links when requests fail to load', async ({ page }) => {
    await page.goto('/login');
    await enterCredentials(page);
    await enterTOTP(page);
    await expect(page.getByText('Не удалось загрузить обращения')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Fixture session' })).toBeVisible();
  });
});
