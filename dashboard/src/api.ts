import { DashboardData, TechnicalIndicators, MarketInfo, PositionInfo, TradeInfo, AlertInfo, StrategyState, StrategyConfig, PricePoint, PerformanceStats, NotificationEvent, NotificationConfig, CostStats, DailyCost, AccountInfo } from './types';

function stripTrailingSlash(value: string): string {
  return value.endsWith('/') ? value.slice(0, -1) : value;
}

function resolveApiBase(): string {
  const override = import.meta.env.VITE_API_BASE_URL?.trim();
  if (override) {
    return `${stripTrailingSlash(override)}/api`;
  }

  if (import.meta.env.DEV) {
    return '/api';
  }

  const { protocol, hostname } = window.location;
  return `${protocol}//${hostname}:9876/api`;
}

function resolveWebSocketUrl(): string {
  const override = import.meta.env.VITE_WS_BASE_URL?.trim();
  if (override) {
    return `${stripTrailingSlash(override)}/ws`;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  if (import.meta.env.DEV) {
    return `${protocol}//${window.location.host}/ws`;
  }

  return `${protocol}//${window.location.hostname}:9876/ws`;
}

const API_BASE = resolveApiBase();
const WS_URL = resolveWebSocketUrl();
const REQUEST_TIMEOUT_MS = 12_000;

class ApiService {
  private ws: WebSocket | null = null;
  private listeners: Map<string, Set<(data: unknown) => void>> = new Map();
  private lastHeartbeat: Date = new Date();

  // 统一错误处理，如果 fetch 失败，则返回特定的错误标记，让 UI 处理显示
  private async safeFetch<T>(url: string, options?: RequestInit): Promise<T> {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

    try {
      const response = await fetch(url, {
        ...options,
        signal: options?.signal ?? controller.signal,
      });
      if (!response.ok) throw new Error(`Fetch failed: ${response.statusText}`);
      return response.json();
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        throw new Error(`Fetch timed out: ${url}`);
      }
      throw error;
    } finally {
      window.clearTimeout(timeout);
    }
  }

  async getDashboard(): Promise<DashboardData> {
    return this.safeFetch<DashboardData>(`${API_BASE}/dashboard`);
  }

  async getMarkets(): Promise<MarketInfo[]> {
    return this.safeFetch<MarketInfo[]>(`${API_BASE}/markets`);
  }

  async getPositions(): Promise<PositionInfo[]> {
    return this.safeFetch<PositionInfo[]>(`${API_BASE}/positions`);
  }

  async getTrades(): Promise<TradeInfo[]> {
    return this.safeFetch<TradeInfo[]>(`${API_BASE}/trades`);
  }

  async getAlerts(): Promise<AlertInfo[]> {
    return this.safeFetch<AlertInfo[]>(`${API_BASE}/alerts`);
  }

  async getIndicators(): Promise<TechnicalIndicators> {
    return this.safeFetch<TechnicalIndicators>(`${API_BASE}/indicators`);
  }

  async getPriceHistory(): Promise<PricePoint[]> {
    return this.safeFetch<PricePoint[]>(`${API_BASE}/price-history`);
  }

  async getStrategy(): Promise<StrategyState> {
    return this.safeFetch<StrategyState>(`${API_BASE}/strategy`);
  }

  async getConfig(): Promise<StrategyConfig> {
    return this.safeFetch<StrategyConfig>(`${API_BASE}/config`);
  }

  async saveConfig(config: StrategyConfig): Promise<StrategyConfig> {
    return this.safeFetch<StrategyConfig>(`${API_BASE}/config`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    });
  }

  async getPerformance(): Promise<PerformanceStats | null> {
    return this.safeFetch<PerformanceStats>(`${API_BASE}/performance`).catch(() => null);
  }

  async getAccount(): Promise<AccountInfo | null> {
    return this.safeFetch<AccountInfo>(`${API_BASE}/account`).catch(() => null);
  }

  async getTradingStopped(): Promise<boolean> {
    const res = await this.safeFetch<{ stopped: boolean }>(`${API_BASE}/trading/stop`);
    return res.stopped;
  }

  async setTradingStopped(stopped: boolean): Promise<boolean> {
    const res = await this.safeFetch<{ stopped: boolean }>(`${API_BASE}/trading/stop`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ stopped }),
    });
    return res.stopped;
  }

  subscribe(type: string, callback: (data: unknown) => void): () => void {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }

    this.listeners.get(type)?.add(callback);

    return () => {
      const callbacks = this.listeners.get(type);
      callbacks?.delete(callback);
      if (callbacks?.size === 0) {
        this.listeners.delete(type);
      }
    };
  }

  connectWebSocket(
    onMessage: (type: string, data: unknown) => void,
    onStatusChange?: (status: 'connected' | 'disconnected' | 'reconnecting' | 'failed') => void
  ) {
    if (this.ws) this.ws.close();

    try {
      this.ws = new WebSocket(WS_URL);
      this.ws.onopen = () => onStatusChange?.('connected');
      this.ws.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          onMessage(message.type, message.data);
          this.listeners.get(message.type)?.forEach(cb => cb(message.data));
        } catch (e) { }
      };
      this.ws.onclose = () => onStatusChange?.('disconnected');
      this.ws.onerror = () => onStatusChange?.('failed');
    } catch (e) {
      onStatusChange?.('failed');
    }
  }

  disconnectWebSocket() {
    if (this.ws) this.ws.close();
    this.ws = null;
  }
}

export const api = new ApiService();
