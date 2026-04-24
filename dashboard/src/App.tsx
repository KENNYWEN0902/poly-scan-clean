import { Component, useState, useEffect, type ErrorInfo, type ReactNode } from 'react';
import { api } from './api';
import { StrategyState, AccountInfo } from './types';
import Portfolio from './pages/Portfolio';
import Strategies from './pages/Strategies';
import Positions from './pages/Positions';
import PnlHistory from './pages/PnlHistory';
import SettingsPage from './pages/Settings';

type PageId = 'dashboard' | 'strategies' | 'positions' | 'history' | 'settings';

interface NavItem {
  id: PageId;
  icon: string;
  label: string;
  sub: string;
}

const navItems: NavItem[] = [
  { id: 'dashboard', icon: 'dashboard', label: '仪表盘', sub: '每日概览' },
  { id: 'strategies', icon: 'insights', label: '策略', sub: '运行中 · 回测' },
  { id: 'positions', icon: 'account_balance_wallet', label: '持仓', sub: '开仓与平仓' },
  { id: 'history', icon: 'history', label: '历史', sub: '交易记录' },
  { id: 'settings', icon: 'settings', label: '设置', sub: '参数配置' },
];

function MIcon({ name, filled, className }: { name: string; filled?: boolean; className?: string }) {
  return (
    <span
      className={`material-symbols-outlined ${className || ''}`}
      style={filled ? { fontVariationSettings: "'FILL' 1" } : undefined}
    >
      {name}
    </span>
  );
}

class PageErrorBoundary extends Component<
  { children: ReactNode; resetKey: string; onReset: () => void },
  { hasError: boolean }
> {
  state = { hasError: false };

  static getDerivedStateFromError() {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[dashboard] page render failed', error, info);
  }

  componentDidUpdate(prevProps: { resetKey: string }) {
    if (prevProps.resetKey !== this.props.resetKey && this.state.hasError) {
      this.setState({ hasError: false });
    }
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-[50vh] flex items-center justify-center">
          <div className="bg-surface-container-lowest border border-white/50 rounded-[2rem] neo-shadow p-8 max-w-md text-center">
            <MIcon name="error" filled className="text-error text-4xl mb-4" />
            <h2 className="text-xl font-black text-slate-900 mb-2">页面加载失败</h2>
            <p className="text-sm text-slate-500 mb-6">这个页面的数据格式异常，仪表盘其他页面仍可继续使用。</p>
            <button
              onClick={this.props.onReset}
              className="px-5 py-3 rounded-xl bg-primary text-white text-sm font-bold hover:bg-primary-container transition-colors"
            >
              返回仪表盘
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}

export default function App() {
  const [activePage, setActivePage] = useState<PageId>('dashboard');
  const [wsStatus, setWsStatus] = useState<'connected' | 'disconnected' | 'reconnecting' | 'failed'>('disconnected');
  const [alertCount, setAlertCount] = useState(0);
  const [strategy, setStrategy] = useState<StrategyState | null>(null);
  const [account, setAccount] = useState<AccountInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    const refreshAccount = () => {
      api.getAccount().then(a => {
        if (!cancelled && a) setAccount(a);
      }).catch(() => {});
    };

    api.getStrategy().then(setStrategy).catch(() => {});
    api.getAlerts().then(alerts => setAlertCount(alerts?.length || 0)).catch(() => {});
    refreshAccount();
    const accountTimer = window.setInterval(refreshAccount, 30_000);
    api.connectWebSocket(
      (type, data) => {
        if (type === 'strategy') setStrategy(data as StrategyState);
        if (type === 'alert') setAlertCount(c => c + 1);
      },
      (status) => setWsStatus(status),
    );
    return () => {
      cancelled = true;
      window.clearInterval(accountTimer);
      api.disconnectWebSocket();
    };
  }, []);

  const activeNav = navItems.find(i => i.id === activePage) || navItems[0];
  const totalPnl = strategy?.total_pnl || 0;
  const collateralBalance =
    account?.collateral_balance ?? account?.pusd_balance ?? account?.usdc_balance ?? 0;
  const portfolioValue = account?.portfolio_value ?? collateralBalance;

  const renderPage = () => {
    switch (activePage) {
      case 'dashboard': return <Portfolio />;
      case 'strategies': return <Strategies />;
      case 'positions': return <Positions />;
      case 'history': return <PnlHistory />;
      case 'settings': return <SettingsPage />;
    }
  };

  return (
    <div className="flex min-h-screen overflow-hidden bg-background text-on-surface">
      {/* Sidebar */}
      <aside className="h-screen w-64 fixed left-0 top-0 flex flex-col bg-slate-50 border-r border-slate-200 z-50">
        <div className="flex flex-col h-full p-4">
          {/* Brand */}
          <div className="flex items-center gap-3 px-2 mb-10">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-primary to-primary-container flex items-center justify-center text-white shadow-lg shadow-primary/20">
              <MIcon name="bolt" filled />
            </div>
            <div>
              <h1 className="text-xl font-black tracking-tight text-slate-900 leading-tight">PolyBot</h1>
              <p className="text-[10px] tracking-widest text-primary font-bold">自动化交易</p>
            </div>
          </div>

          {/* Nav */}
          <nav className="flex-1 space-y-1">
            {navItems.map(item => {
              const isActive = item.id === activePage;
              return (
                <button
                  key={item.id}
                  onClick={() => setActivePage(item.id)}
                  className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl active:scale-[0.98] transition-all text-left ${
                    isActive
                      ? 'text-violet-700 font-semibold bg-violet-50'
                      : 'text-slate-600 hover:text-slate-900 hover:bg-slate-200/50'
                  }`}
                >
                  <MIcon name={item.icon} />
                  <div className="flex flex-col">
                    <span className="text-sm">{item.label}</span>
                    {isActive && (
                      <span className="text-[10px] opacity-70 font-normal">{item.sub}</span>
                    )}
                  </div>
                </button>
              );
            })}
          </nav>

          {/* Sidebar footer */}
          <div className="mt-auto pt-6 space-y-4">
            {/* Global Equity mini chart */}
            <div className="p-4 bg-surface-container rounded-2xl">
              <div className="flex justify-between items-end mb-2">
                <span className="text-[10px] text-slate-500 font-bold uppercase">总权益</span>
                <span className={`text-xs font-bold ${totalPnl >= 0 ? 'text-secondary' : 'text-error'}`}>
                  {totalPnl >= 0 ? '+' : ''}${totalPnl.toFixed(2)}
                </span>
              </div>
              <div className="h-8 w-full bg-slate-200/50 rounded flex items-end overflow-hidden">
                <div className="w-full h-full bg-gradient-to-t from-secondary/10 to-transparent flex items-end">
                  <svg className="w-full h-6" preserveAspectRatio="none" viewBox="0 0 100 20">
                    {account?.equity_curve?.length ? (() => {
                      const pts = account.equity_curve;
                      const min = Math.min(...pts.map(p => p.value));
                      const max = Math.max(...pts.map(p => p.value));
                      const range = max - min || 1;
                      const d = pts.map((p, i) => {
                        const x = (i / Math.max(pts.length - 1, 1)) * 100;
                        const y = 18 - ((p.value - min) / range) * 16;
                        return `${i === 0 ? 'M' : 'L'}${x},${y}`;
                      }).join(' ');
                      return <path d={d} fill="none" stroke="#10B981" strokeWidth="2" />;
                    })() : (
                      <path d="M0,18 Q15,10 30,14 T55,8 T80,12 T100,6" fill="none" stroke="#10B981" strokeWidth="2" />
                    )}
                  </svg>
                </div>
              </div>
              <div className="mt-2 text-lg font-black text-slate-900">${portfolioValue.toFixed(2)}</div>
            </div>

            {/* User profile */}
            <div className="flex items-center gap-3 p-2 bg-white rounded-2xl shadow-sm">
              <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-primary to-primary-container flex items-center justify-center text-white">
                <MIcon name="person" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="text-xs font-bold truncate">PolyBot Engine</div>
                <div className="flex items-center gap-1">
                  <span className={`w-1.5 h-1.5 rounded-full ${wsStatus === 'connected' ? 'bg-secondary animate-pulse' : 'bg-error'}`} />
                  <span className="text-[10px] text-slate-500 uppercase font-medium">
                    {wsStatus === 'connected' ? '在线 · 已同步' : '离线'}
                  </span>
                </div>
              </div>
              <button className="text-slate-400 hover:text-error transition-colors">
                <MIcon name="logout" className="text-lg" />
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* Main content wrapper */}
      <main className="ml-64 flex-1 flex flex-col min-h-screen relative overflow-y-auto">
        {/* Top navbar */}
        <header className="fixed top-0 right-0 w-[calc(100%-16rem)] h-16 bg-white/80 backdrop-blur-xl z-40 border-b border-slate-100">
          <div className="flex justify-between items-center px-8 h-full">
            <div className="flex items-center gap-2 text-xs font-medium text-slate-400">
              <span>页面</span>
              <span className="text-slate-300">/</span>
              <span>{activeNav.label}</span>
              <span className="text-slate-300">/</span>
              <span className="text-slate-900 font-semibold">{activeNav.sub}</span>
            </div>
            <div className="flex items-center gap-6">
              <div className="relative hidden lg:block">
                <MIcon name="search" className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400 text-lg" />
                <input
                  type="text"
                  placeholder="搜索市场..."
                  className="bg-surface-container-low border-none rounded-xl pl-10 pr-4 py-2 text-sm w-64 focus:ring-2 focus:ring-primary/20 transition-all outline-none"
                />
              </div>
              <div className="flex items-center gap-2">
                <button className="p-2 text-slate-500 hover:bg-slate-100 rounded-lg transition-colors relative">
                  <MIcon name="notifications" />
                  {alertCount > 0 && (
                    <span className="absolute top-1.5 right-1.5 w-4 h-4 bg-primary text-white text-[10px] flex items-center justify-center rounded-full border-2 border-white">
                      {alertCount > 9 ? '9+' : alertCount}
                    </span>
                  )}
                </button>
                <button
                  onClick={() => setActivePage('settings')}
                  className="p-2 text-slate-500 hover:bg-slate-100 rounded-lg transition-colors"
                >
                  <MIcon name="settings" />
                </button>
              </div>
            </div>
          </div>
        </header>

        {/* Page content */}
        <div className="mt-16 p-8 pb-12 animate-fade-in">
          <PageErrorBoundary resetKey={activePage} onReset={() => setActivePage('dashboard')}>
            {renderPage()}
          </PageErrorBoundary>
        </div>

        {/* Status bar */}
        <footer className="fixed bottom-0 right-0 w-[calc(100%-16rem)] h-8 bg-primary/95 text-white flex items-center justify-between px-8 text-[10px] font-bold tracking-widest uppercase z-50">
          <div className="flex items-center gap-2">
            <span className={`w-1.5 h-1.5 rounded-full ${wsStatus === 'connected' ? 'bg-secondary-fixed' : 'bg-error'}`} />
            系统状态：{wsStatus === 'connected' ? '已同步并正常运行' : '已断开'}
          </div>
          <div className="flex items-center gap-4">
            <span>交易数：{strategy?.total_trades || 0}</span>
            <span className="text-primary-fixed">
              胜率：{(strategy?.win_rate || 0).toFixed(1)}%
            </span>
          </div>
        </footer>
      </main>
    </div>
  );
}
