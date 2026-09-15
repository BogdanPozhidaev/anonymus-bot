import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  scenarios: {
    fifty_sessions: {
      executor: 'per-vu-iterations',
      vus: 50,
      iterations: 5,
      maxDuration: '2m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<1000'], // 95% запросов быстрее 1 секунды
    http_req_failed: ['rate<0.01'],    // менее 1% ошибок
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  // __VU — номер виртуального пользователя (1..50), сопоставляем с seed-данными
  const senderTelegramID = 900000000 + __VU * 2;

  const payload = JSON.stringify({
    sender_telegram_id: senderTelegramID,
    content_type: 'text',
    content: `Load test message ${__ITER} from VU ${__VU}`,
  });

  const params = {
    headers: { 'Content-Type': 'application/json' },
  };

  const res = http.post(`${BASE_URL}/internal/messages/relay`, payload, params);

  check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 1000ms': (r) => r.timings.duration < 1000,
    'not blocked': (r) => {
      const body = JSON.parse(r.body);
      return body.blocked === false;
    },
  });

  sleep(1); // имитация реального интервала между сообщениями пользователя
}