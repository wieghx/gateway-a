// Gateway-a Concurrent Connection Stress Test
// Tests 10,000 concurrent connections simultaneously
// This is a focused test for maximum connection handling

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter, Trend, Gauge } from 'k6/metrics';
import { concurrentConnection, activeConnections } from 'k6/x/wasm';

// Custom metrics
const concurrentGauge = new Gauge('concurrent_connections');
const connectionSuccessRate = new Rate('connection_success');
const connectionLatency = new Trend('connection_latency');
const connectionErrors = new Counter('connection_errors');
const activeUsersGauge = new Gauge('active_users');

export const options = {
  // 10,000 concurrent connections test
  scenarios: {
    // All connections start simultaneously (worst-case scenario)
    spike: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '5s', target: 10000 },  // Instant spike to 10k
        { duration: '60s', target: 10000 }, // Hold for 1 minute
        { duration: '30s', target: 0 },     // Immediate drop
      ],
      exec: 'spikeTest',
      gracefulStop: '2s',
      maxVUs: 10000,
    },

    // Gradual ramp-up with connection tracking
    gradual: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '10s', target: 1000 },
        { duration: '15s', target: 3000 },
        { duration: '15s', target: 5000 },
        { duration: '15s', target: 7500 },
        { duration: '20s', target: 10000 },
        { duration: '120s', target: 10000 }, // 2 minutes stability test
        { duration: '30s', target: 0 },
      ],
      exec: 'gradualTest',
      gracefulStop: '5s',
    },

    // Sustained high load
    sustained: {
      executor: 'constant-vus',
      vus: 10000,
      duration: '2m',
      exec: 'sustainedTest',
    },
  },

  thresholds: {
    // Must handle all connections
    connection_success: ['rate>0.95'],

    // Latency should be reasonable even under load
    connection_latency: ['p95<3000', 'p99<5000'],

    // Error rate should stay below 5%
    connection_errors: ['rate<0.05'],

    // Overall HTTP success rate
    http_req_status: ['rate>0.90'],
    http_req_duration: ['p95<2000'],
  },

  // Handle high load settings
  noConnectionReuse: false,
  discardResponseBodies: true,
};

const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || 'concurrent-test-key';

// Simulate different client IPs for realistic load testing
function getClientIP(index) {
  return `192.168.${Math.floor(index / 256)}.${index % 256}`;
}

// Spike test - instant 10k connections
export function spikeTest() {
  const vuid = __VU;
  const clientIP = getClientIP(vuid);

  const params = {
    headers: {
      'X-Client-IP': clientIP,
      'Authorization': `Bearer ${API_KEY}`,
    },
    timeout: '5s',
  };

  const start = new Date().getTime();
  const response = http.get(`${BASE_URL}/health`, params);
  const latency = new Date().getTime() - start;

  concurrentGauge.add(1);
  activeUsersGauge.add(1);
  connectionLatency.add(latency);

  const passed = check(response, {
    'spike test connection successful': (r) => r.status === 200 || r.status === 429,
    'spike test fast response': (r) => r.timings.duration < 5000,
  });

  connectionSuccessRate.add(passed ? 1 : 0);
  activeConnections.add(1);

  if (!passed) {
    connectionErrors.add(1);
  }

  // Quick sleep to maintain connection
  sleep(0.5);

  activeConnections.sub(1);
  activeUsersGauge.sub(1);
}

// Gradual test - track connections as they ramp up
export function gradualTest() {
  const vuid = __VU;
  const clientIP = getClientIP(vuid);

  const params = {
    headers: {
      'X-Client-IP': clientIP,
      'Authorization': `Bearer ${API_KEY}`,
    },
    timeout: '5s',
  };

  const response = http.get(`${BASE_URL}/health`, params);

  const passed = check(response, {
    'gradual test connection successful': (r) => r.status >= 200 && r.status < 400,
    'gradual test response time acceptable': (r) => r.timings.duration < 3000,
  });

  connectionSuccessRate.add(passed ? 1 : 0);
  connectionLatency.add(response.timings.duration);

  if (!passed) {
    connectionErrors.add(1);
  }

  sleep(0.2 + (Math.random() * 0.3));
}

// Sustained test - constant high load
export function sustainedTest() {
  const vuid = __VU;
  const clientIP = getClientIP(vuid);

  const params = {
    headers: {
      'X-Client-IP': clientIP,
      'Authorization': `Bearer ${API_KEY}`,
    },
    timeout: '5s',
  };

  const response = http.get(`${BASE_URL}/health`, params);

  const passed = check(response, {
    'sustained test connection successful': (r) => r.status >= 200,
    'sustained test within latency': (r) => r.timings.duration < 4000,
  });

  connectionSuccessRate.add(passed ? 1 : 0);
  connectionLatency.add(response.timings.duration);

  if (!passed) {
    connectionErrors.add(1);
  }

  // Short sleep to maintain high concurrency
  sleep(0.1);
}

// Health endpoint test
export function healthEndpoint() {
  const response = http.get(`${BASE_URL}/health`, {
    headers: { 'Accept': 'application/json' },
  });

  check(response, {
    'health status 200': (r) => r.status === 200,
    'health has ok': (r) => r.body.includes('"status":"ok"'),
  });

  sleep(0.5);
}

// Root endpoint test
export function rootEndpoint() {
  const response = http.get(BASE_URL, {
    headers: { 'Accept': 'application/json' },
  });

  check(response, {
    'root returns 200': (r) => r.status === 200,
    'root has service name': (r) => r.body.includes('gateway-a'),
  });

  sleep(0.3);
}

// Metrics report at regular intervals
export function metricReport() {
  // This function is called periodically during test
  // for custom metrics reporting
  return {
    current_connections: activeConnections.value,
    total_errors: connectionErrors.value,
  };
}

// Setup for test initialization
export function setup() {
  console.log('='.repeat(60));
  console.log('GATEWAY-A CONCURRENT CONNECTION TEST');
  console.log('='.repeat(60));
  console.log(`Target: ${BASE_URL}`);
  console.log('Maximum VUs: 10000');
  console.log('');
  console.log('This test simulates 10,000 concurrent connections');
  console.log('to validate the gateway connection handling capacity.');
  console.log('='.repeat(60));
}

// Cleanup and summary
export function teardown() {
  console.log('');
  console.log('Test completed. Cleaning up...');
}

// Custom summary with detailed metrics
export function handleSummary(data) {
  console.log('');
  console.log('='.repeat(60));
  console.log('CONCURRENT CONNECTION TEST RESULTS');
  console.log('='.repeat(60));

  // Key metrics
  const successRate = data.metrics.connection_success_rate?.values.rate || 0;
  const errorCount = data.metrics.connection_errors?.values.count || 0;
  const latencyP95 = data.metrics.connection_latency?.values.p95 || 0;
  const latencyP99 = data.metrics.connection_latency?.values.p99 || 0;

  console.log(`Success Rate: ${(successRate * 100).toFixed(2)}%`);
  console.log(`Connection Errors: ${errorCount}`);
  console.log(`Latency P95: ${latencyP95.toFixed(2)}ms`);
  console.log(`Latency P99: ${latencyP99.toFixed(2)}ms`);

  // HTTP metrics
  if (data.metrics.http_req_duration) {
    console.log('');
    console.log('HTTP Request Duration:');
    console.log(`  P50: ${data.metrics.http_req_duration.values.p50.toFixed(2)}ms`);
    console.log(`  P90: ${data.metrics.http_req_duration.values.p90.toFixed(2)}ms`);
    console.log(`  P95: ${data.metrics.http_req_duration.values.p95.toFixed(2)}ms`);
    console.log(`  P99: ${data.metrics.http_req_duration.values.p99.toFixed(2)}ms`);
  }

  console.log('='.repeat(60));

  return {
    'concurrent-test-results.txt': formatResults(data),
    'concurrent-test-summary.json': JSON.stringify(data, null, 2),
  };
}

function formatResults(data) {
  let result = '';
  result += 'Concurrent Connection Test Results\n';
  result += '='.repeat(40) + '\n';
  result += `Date: ${new Date().toISOString()}\n`;
  result += `Target: ${BASE_URL}\n`;
  result += `Max VUs: 10000\n\n`;

  if (data.metrics.connection_success_rate) {
    result += `Connection Success Rate: ${(data.metrics.connection_success_rate.values.rate * 100).toFixed(2)}%\n`;
  }

  if (data.metrics.connection_errors) {
    result += `Connection Errors: ${data.metrics.connection_errors.values.count}\n`;
  }

  if (data.metrics.connection_latency) {
    result += `Connection Latency P95: ${data.metrics.connection_latency.values.p95.toFixed(2)}ms\n`;
    result += `Connection Latency P99: ${data.metrics.connection_latency.values.p99.toFixed(2)}ms\n`;
  }

  return result;
}
