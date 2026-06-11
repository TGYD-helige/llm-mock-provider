import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '2m', target: 100 },
    { duration: '10m', target: 100 },
    { duration: '2m', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<30000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:3001';
const API_KEY = __ENV.API_KEY || 'dummy-key';
const MODEL = __ENV.MODEL || 'mock-gpt';
const QUERY = __ENV.QUERY || '?ttft_ms=300&chunk_delay_ms=50&completion_tokens=200';

export default function () {
  const body = JSON.stringify({
    model: MODEL,
    messages: [{ role: 'user', content: 'hello' }],
    stream: true,
  });

  const res = http.post(`${BASE_URL}/v1/chat/completions${QUERY}`, body, {
    headers: {
      Authorization: `Bearer ${API_KEY}`,
      'Content-Type': 'application/json',
    },
    timeout: '120s',
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
    'contains data': (r) => r.status === 200 && r.body && r.body.includes('data:'),
    'contains DONE': (r) => r.status === 200 && r.body && r.body.includes('[DONE]'),
  });

  sleep(1);
}
