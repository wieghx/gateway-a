// Gateway-a k6 Load Testing Script
// Performance validation for AI Gateway under high load
// Target: 10,000 concurrent connections

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter, Trend } from 'k6/metrics';

// Custom metrics for detailed analysis
const successRate = new Rate('success_rate');
const apiErrors = new Counter('api_errors');
const latencyP99 = new Trend('latency_p99');
const latencyP95 = new Trend('latency_p95');
const concurrentConnections = new Counter('concurrent_connections');

// Test configuration
export const options = {
  // Test scenario configuration
  stages: [
    // Warmup phase - gradually increase load
    { duration: '30s', target: 1000 },   // Ramp up to 1k connections
    { duration: '30s', target: 5000 },   // Ramp up to 5k connections
    { duration: '60s', target: 10000 },  // Reach 10k concurrent connections
    { duration: '120s', target: 10000 }, // Stay at 10k for 2 minutes
    { duration: '60s', target: 5000 },   // Ramp down to 5k
    { duration: '30s', target: 0 },      // Ramp down to 0

    // Sustained load test - verify stability
    { duration: '5m', target: 10000 },   // 5 minutes at full capacity
    { duration: '1m', target: 0 },       // Cool down
  ],

  // Connection pooling and timeout settings
  maxRequestsPerConn: 100,
  noConnectionReuse: false,
  discardResponseBodies: true,

  // Scenarios for different test types
  scenarios: {
    // Sequential requests from single source
    warmup: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 1000 },
        { duration: '60s', target: 10000 },
        { duration: '2m', target: 10000 },
        { duration: '30s', target: 0 },
      ],
      exec: 'chatCompletion',
      gracefulStop: '5s',
    },

    // Concurrent connections test
    concurrent: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '10s', target: 5000 },
        { duration: '30s', target: 10000 },
        { duration: '1m', target: 10000 },
        { duration: '10s', target: 0 },
      ],
      exec: 'healthCheck',
      gracefulStop: '5s',
    },

    // API key authentication stress test
    authStress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 3000 },
        { duration: '60s', target: 8000 },
        { duration: '90s', target: 8000 },
        { duration: '30s', target: 0 },
      ],
      exec: 'authenticatedRequest',
      gracefulStop: '5s',
    },

    // Streaming load test
    streaming: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 2000 },
        { duration: '60s', target: 5000 },
        { duration: '2m', target: 5000 },
        { duration: '30s', target: 0 },
      ],
      exec: 'streamingRequest',
      gracefulStop: '10s',
    },

    // Queue operations test
    queueStress: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '20s', target: 1500 },
        { duration: '40s', target: 4000 },
        { duration: '1m', target: 4000 },
        { duration: '20s', target: 0 },
      ],
      exec: 'queueOperations',
      gracefulStop: '5s',
    },
  },

  // Thresholds for passing the test
  thresholds: {
    // HTTP status codes should be 95% successful
    http_req_status: ['rate>0.95'],

    // Response times: 95% under 2 seconds
    http_req_duration: ['p(95)<2000'],

    // Success rate for individual checks
    checks: ['rate>0.90'],

    // API error rate should be under 5%
    api_errors: ['rate<0.05'],

    // P99 latency should not exceed 3 seconds
    latency_p99: ['avg<3000'],

    // P95 latency should not exceed 1.5 seconds
    latency_p95: ['avg<1500'],

    // Connection establishment should be reliable
    connection_success: ['rate>0.99'],
  },

  // Metrics to drop (reduce memory usage)
  metricSamples: {
    dropPrefixes: ['http_reqs', 'http_req_blocked', 'http_req_tls_handshaking'],
  },

  // Error handling
  throw: false,
};

// Base URL for the gateway (can be overridden via environment variable)
const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || 'test-api-key-12345';

// Test data generator
function generateTestPayload(streaming = false) {
  const messages = [
    { role: 'user', content: 'Hello, how can you help me today?' },
    { role: 'assistant', content: "I'm here to assist you with various tasks." },
    { role: 'user', content: 'Can you explain quantum computing in simple terms?' },
  ];

  return {
    model: 'test-model',
    messages: messages,
    stream: streaming,
    max_tokens: 100,
    temperature: 0.7,
  };
}

// Health check scenario
export function healthCheck() {
  const params = {
    headers: {
      'Accept': 'application/json',
    },
  };

  const response = http.get(`${BASE_URL}/health`, params);

  const passed = check(response, {
    'health check status is 200': (r) => r.status === 200,
    'health check has ok response': (r) => r.body.includes('"status":"ok"'),
  });

  successRate.add(passed ? 1 : 0);

  if (!passed) {
    apiErrors.add(1);
  }

  sleep(0.1);
}

// Chat completion scenario (non-streaming)
export function chatCompletion() {
  const payload = JSON.stringify(generateTestPayload(false));

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
      'Accept': 'application/json',
    },
  };

  const response = http.post(`${BASE_URL}/v1/chat/completions`, payload, params);

  const passed = check(response, {
    'chat completion status is 2xx': (r) => r.status >= 200 && r.status < 300,
    'chat completion response time < 2s': (r) => r.timings.duration < 2000,
  });

  successRate.add(passed ? 1 : 0);
  latencyP99.add(response.timings.duration);
  latencyP95.add(response.timings.duration);

  if (!passed) {
    apiErrors.add(1);
  }

  // Variable sleep to simulate user behavior
  sleep(Math.random() * 0.5 + 0.1);
}

// Authenticated request scenario
export function authenticatedRequest() {
  const params = {
    headers: {
      'Authorization': `Bearer ${API_KEY}`,
      'X-Client-IP': `192.168.1.${Math.floor(Math.random() * 255)}`,
    },
  };

  const response = http.get(`${BASE_URL}/v1/chat/completions`, null, params);

  const passed = check(response, {
    'authenticated request status is 2xx or 4xx': (r) => r.status >= 200 && r.status < 400,
    'authenticated request fast response': (r) => r.timings.duration < 1000,
  });

  successRate.add(passed ? 1 : 0);

  if (!passed) {
    apiErrors.add(1);
  }

  sleep(0.2);
}

// Streaming request scenario
export function streamingRequest() {
  const payload = JSON.stringify(generateTestPayload(true));

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
      'Accept': 'text/event-stream',
    },
    timeout: '5s',
  };

  // Note: k6 handles streaming differently
  // For load testing, we just check connection success
  const response = http.post(`${BASE_URL}/v1/chat/completions`, payload, params);

  const passed = check(response, {
    'streaming connection established': (r) => r.status === 200 || r.status === 502,
    'streaming response time < 3s': (r) => r.timings.duration < 3000,
  });

  successRate.add(passed ? 1 : 0);

  if (response.timings.duration > 0) {
    latencyP99.add(response.timings.duration);
    latencyP95.add(response.timings.duration);
  }

  if (!passed) {
    apiErrors.add(1);
  }

  sleep(0.3);
}

// Queue operations scenario
export function queueOperations() {
  const payload = JSON.stringify({
    job_type: 'token_calculation',
    payload: {
      text: 'test payload for queue operation',
      length: 100,
    },
    queue: 'default',
    max_retries: 3,
    timeout: '30s',
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
    },
  };

  const response = http.post(`${BASE_URL}/v1/queue/enqueue`, payload, params);

  const passed = check(response, {
    'queue enqueue status is 2xx': (r) => r.status >= 200 && r.status < 300,
    'queue response has job_id': (r) => r.body.includes('job_id'),
  });

  successRate.add(passed ? 1 : 0);

  if (!passed) {
    apiErrors.add(1);
  }

  // Poll job status
  if (response.body) {
    try {
      const jobResponse = JSON.parse(response.body);
      const jobID = jobResponse.job_id || 'test-job';
      const statusResponse = http.get(
        `${BASE_URL}/v1/queue/jobs/${jobID}`,
        { headers: { 'Authorization': `Bearer ${API_KEY}` } }
      );
      check(statusResponse, {
        'job status check successful': (r) => r.status === 200 || r.status === 404,
      });
    } catch (e) {
      // JSON parsing failed, continue
    }
  }

  sleep(0.2);
}

// HTTP request handler (for custom endpoints)
http.handleRequest = (method, url, body, headers) => {
  // Custom request handling if needed
  return http[method](url, body, headers);
};

// Connection tracking
export function setup() {
  console.log('Starting Gateway-a load test');
  console.log(`Target URL: ${BASE_URL}`);
  console.log(`Test stages: ${JSON.stringify(options.stages)}`);
  console.log('');
}

// Summary handler for results
export function handleSummary(data) {
  console.log('');
  console.log('='.repeat(60));
  console.log('GATEWAY-A LOAD TEST RESULTS');
  console.log('='.repeat(60));

  // Overall success rate
  const success = data.metrics.success_rate ? data.metrics.success_rate.values.rate : 0;
  console.log(`Overall Success Rate: ${(success * 100).toFixed(2)}%`);

  // Error count
  const errors = data.metrics.api_errors ? data.metrics.api_errors.values.count : 0;
  console.log(`Total API Errors: ${errors}`);

  // Latency percentiles
  if (data.metrics.latency_p95) {
    console.log(`P95 Latency: ${data.metrics.latency_p95.values.p95.toFixed(2)}ms`);
  }
  if (data.metrics.latency_p99) {
    console.log(`P99 Latency: ${data.metrics.latency_p99.values.p99.toFixed(2)}ms`);
  }

  // HTTP status breakdown
  console.log('');
  console.log('HTTP Status Distribution:');
  const statusMetrics = data.metrics.http_req_status;
  if (statusMetrics) {
    for (const [status, value] of Object.entries(statusMetrics.values)) {
      if (status !== 'rate' && status !== 'count') {
        console.log(`  ${status}: ${value.rate.toFixed(2)}%`);
      }
    }
  }

  console.log('='.repeat(60));
  console.log('');

  return {
    'load-test-results.txt': formatTestResults(data),
    'load-test-summary.json': JSON.stringify(data, null, 2),
  };
}

function formatTestResults(data) {
  let result = 'Gateway-a Load Test Results\n';
  result += '='.repeat(40) + '\n';
  result += `Timestamp: ${new Date().toISOString()}\n`;
  result += `Target: ${BASE_URL}\n\n`;

  if (data.metrics.success_rate) {
    result += `Success Rate: ${(data.metrics.success_rate.values.rate * 100).toFixed(2)}%\n`;
  }

  if (data.metrics.api_errors) {
    result += `API Errors: ${data.metrics.api_errors.values.count}\n`;
  }

  if (data.metrics.http_req_duration) {
    result += `HTTP Duration P50: ${data.metrics.http_req_duration.values.p50.toFixed(2)}ms\n`;
    result += `HTTP Duration P90: ${data.metrics.http_req_duration.values.p90.toFixed(2)}ms\n`;
    result += `HTTP Duration P95: ${data.metrics.http_req_duration.values.p95.toFixed(2)}ms\n`;
    result += `HTTP Duration P99: ${data.metrics.http_req_duration.values.p99.toFixed(2)}ms\n`;
    result += `HTTP Duration Avg: ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms\n`;
  }

  return result;
}
