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
    http_req_duration: ['p(95)<1000'], // 95% of requests complete within one second
    http_req_failed: ['rate<0.01'],    // Fewer than 1% of requests fail
    checks: ['rate==1'],
  },
};

const BASE_URL = __ENV.BASE_URL;
if (!/^http:\/\/(?:127\.0\.0\.1|localhost)(?::[1-9][0-9]{0,4})?$/.test(BASE_URL)) {
  throw new Error('BASE_URL must be a loopback HTTP origin for the disposable test');
}
if (!__ENV.INTERNAL_API_TOKEN_FILE) {
  throw new Error('INTERNAL_API_TOKEN_FILE must point to the disposable test token');
}
const internalToken = open(__ENV.INTERNAL_API_TOKEN_FILE).trim();
if (!/^[0-9a-fA-F]{64}$/.test(internalToken)) {
  throw new Error('INTERNAL_API_TOKEN_FILE must contain a 64-character hex token');
}

export default function () {
  // Map the virtual user number to the seeded fixtures.
  const senderTelegramID = 900000000 + __VU * 2;

  const payload = JSON.stringify({
    sender_telegram_id: senderTelegramID,
    content_type: 'text',
    content: `Load test message ${__ITER} from VU ${__VU}`,
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'X-Internal-Token': internalToken,
    },
    redirects: 0,
  };

  const res = http.post(`${BASE_URL}/internal/messages/relay`, payload, params);

  check(res, {
    'status is 200': (r) => r.status === 200,
    'message queued': (r) => {
      if (r.status !== 200) return false;
      try {
        const body = r.json();
        return body?.blocked === false && body.message_id > 0 && body.session_id > 0;
      } catch {
        return false;
      }
    },
  });

  sleep(1); // Simulate a gap between user messages.
}
