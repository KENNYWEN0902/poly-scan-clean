const fs = require('fs');
const path = require('path');

// Load .env file manually (PM2 doesn't support dotenv natively)
function loadEnv(envPath) {
  const env = {};
  try {
    const content = fs.readFileSync(envPath, 'utf8');
    for (const line of content.split('\n')) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;
      const [key, ...rest] = trimmed.split('=');
      if (key && rest.length > 0) {
        env[key.trim()] = rest.join('=').trim();
      }
    }
  } catch (e) {
    console.error('Warning: .env file not found at', envPath);
  }
  return env;
}

const dotenv = loadEnv(path.join(__dirname, '.env'));

module.exports = {
  apps: [
    // BTC 5分钟延迟套利策略
    {
      name: 'poly-bot-btc',
      script: './poly-bot',
      cwd: '/root/poly-scan',
      watch: false,
      autorestart: true,
      restart_delay: 5000,
      max_restarts: 50,
      min_uptime: '30s',
      env: {
        // From .env file
        POLY_PRIVATE_KEY: dotenv.POLY_PRIVATE_KEY || '',
        POLY_SIGNATURE_TYPE: dotenv.POLY_SIGNATURE_TYPE || '2',
        POLY_FUNDER_ADDRESS: dotenv.POLY_FUNDER_ADDRESS || '',
        POLY_CLOB_HOST: dotenv.POLY_CLOB_HOST || '',
        POLY_BUILDER_CODE: dotenv.POLY_BUILDER_CODE || '',
        POLY_RPC_URL: dotenv.POLY_RPC_URL || '',
        POLY_TAKE_PROFIT_PCT: dotenv.POLY_TAKE_PROFIT_PCT || '0.50',
        POLY_STOP_LOSS_PCT: dotenv.POLY_STOP_LOSS_PCT || '0.30',
        POLY_SIMULATION: dotenv.POLY_SIMULATION || '0', // Simulation mode: set to '0' for live trading
        POLY_REQUIRE_CHAINLINK_RTDS: dotenv.POLY_REQUIRE_CHAINLINK_RTDS || '0',
        POLY_RELAYER_API_KEY: dotenv.POLY_RELAYER_API_KEY || '',
        // CRITICAL: Do NOT set API_KEY/SECRET/PASSPHRASE — let Python derive fresh ones
        POLY_API_KEY: '',
        POLY_API_SECRET: '',
        POLY_PASSPHRASE: '',
      },
      error_file: '/root/poly-scan/logs/btc-error.log',
      out_file: '/root/poly-scan/logs/btc-out.log',
      time: true,
      merge_logs: true,
    },
    // Dashboard API 服务器
    {
      name: 'poly-bot-api',
      script: './poly-bot-api',
      cwd: '/root/poly-scan',
      watch: false,
      autorestart: true,
      restart_delay: 3000,
      max_restarts: 10,
      env: {
        API_PORT: 9876,
        API_BIND_HOST: dotenv.API_BIND_HOST || '0.0.0.0',
        POLY_PRIVATE_KEY: dotenv.POLY_PRIVATE_KEY || '',
        POLY_SIGNATURE_TYPE: dotenv.POLY_SIGNATURE_TYPE || '2',
        POLY_FUNDER_ADDRESS: dotenv.POLY_FUNDER_ADDRESS || '',
        POLY_ADDRESS: dotenv.POLY_ADDRESS || '',
        POLY_CLOB_HOST: dotenv.POLY_CLOB_HOST || '',
        POLY_BUILDER_CODE: dotenv.POLY_BUILDER_CODE || '',
        POLY_RPC_URL: dotenv.POLY_RPC_URL || '',
        POLYGON_RPC_URL: dotenv.POLYGON_RPC_URL || '',
        POLY_DASHBOARD_FETCH_GENERIC_MARKETS: dotenv.POLY_DASHBOARD_FETCH_GENERIC_MARKETS || '0',
        // CRITICAL: Do NOT set API_KEY/SECRET/PASSPHRASE — let Python derive fresh ones
        POLY_API_KEY: '',
        POLY_API_SECRET: '',
        POLY_PASSPHRASE: '',
      },
      error_file: '/root/poly-scan/logs/api-error.log',
      out_file: '/root/poly-scan/logs/api-out.log',
      time: true,
      merge_logs: true,
    },
    // Dashboard 前端 (static build served via serve)
    {
      name: 'poly-dashboard',
      script: 'npx',
      args: 'serve dist -l tcp://0.0.0.0:3457 -s',
      cwd: '/root/poly-scan/dashboard',
      watch: false,
      autorestart: true,
      restart_delay: 3000,
      max_restarts: 10,
      env: {
        NODE_ENV: 'production',
      },
      error_file: '/root/poly-scan/logs/dashboard-error.log',
      out_file: '/root/poly-scan/logs/dashboard-out.log',
      time: true,
      merge_logs: true,
    }
  ]
};
