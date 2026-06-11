import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 100,
  duration: '1h',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<2000', 'p(99)<5000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:3001';
const API_KEY = __ENV.API_KEY || 'dummy-key';
const MODEL = __ENV.MODEL || 'mock-gpt';
const QUERY = __ENV.QUERY || '';

export default function () {
  const body = JSON.stringify({
    model: MODEL,
    messages: [{ role: 'user', content: 'hello' }],
    stream: false,
  });

  const res = http.post(`${BASE_URL}/v1/chat/completions${QUERY}`, body, {
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      'Content-Type': 'application/json',
    },
    timeout: '60s',
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'has usage': (r) => r.json('usage') !== undefined,
  });

  sleep(1);
}
