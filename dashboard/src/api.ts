import { DashboardData, TechnicalIndicators, MarketInfo, PositionInfo, TradeInfo, AlertInfo, StrategyState, StrategyConfig, PricePoint, PerformanceStats, NotificationEvent, NotificationConfig, CostStats, DailyCost, AccountInfo } from './types';

const API_BASE = '/api';

class ApiService {
  private ws: WebSocket | null = null;
  private listeners: Map<string, Set<(data: unknown) => void>> = new Map();
  private lastHeartbeat: Date = new Date();

  // 统一错误处理，如果 fetch 失败，则返回特定的错误标记，让 UI 处理显示
  private async safeFetch<T>(url: string, options?: RequestInit): Promise<T> {
    const response = await fetch(url, options);
    if (!response.ok) throw new Error(`Fetch failed: ${response.statusText}`);
    return response.json();
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

  connectWebSocket(
    onMessage: (type: string, data: unknown) => void,
    onStatusChange?: (status: 'connected' | 'disconnected' | 'reconnecting' | 'failed') => void
  ) {
    if (this.ws) this.ws.close();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    try {
      this.ws = new WebSocket(wsUrl);
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
