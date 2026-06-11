import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<2000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:3001';
const API_KEY = __ENV.API_KEY || 'dummy-key';
const MODEL = __ENV.MODEL || 'mock-gpt';

export default function () {
  const body = JSON.stringify({
    model: MODEL,
    messages: [{ role: 'user', content: 'hello' }],
    stream: false,
  });

  const res = http.post(`${BASE_URL}/v1/chat/completions`, body, {
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      'Content-Type': 'application/json',
    },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'has choices': (r) => Array.isArray(r.json('choices')) && r.json('choices').length > 0,
    'has usage': (r) => r.json('usage') !== undefined,
  });

  sleep(1);
}
