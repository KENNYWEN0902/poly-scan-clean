import { useState, useEffect } from 'react';
import { api } from '../api';
import { PositionInfo, AccountInfo } from '../types';

function isPositiveOutcome(side: string) {
  return ['YES', 'BUY', 'UP'].includes(side.toUpperCase());
}

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

export default function Positions() {
  const [positions, setPositions] = useState<PositionInfo[]>([]);
  const [account, setAccount] = useState<AccountInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<'all' | 'active' | 'closed'>('all');

  useEffect(() => {
    let cancelled = false;

    api.getPositions().catch(() => []).then(p => {
      if (cancelled) return;
      setPositions(Array.isArray(p) ? p : []);
      setLoading(false);
    });

    api.getAccount().then(a => {
      if (!cancelled && a) setAccount(a);
    }).catch(() => {});

    return () => {
      cancelled = true;
    };
  }, []);

  const filtered = positions.filter(p => {
    if (filter === 'active') return p.is_active;
    if (filter === 'closed') return !p.is_active;
    return true;
  });

  const totalPnl = filtered.reduce((sum, p) => sum + p.pnl, 0);
  const activeCount = positions.filter(p => p.is_active).length;
  const closedCount = positions.filter(p => !p.is_active).length;
  const collateralBalance =
    account?.collateral_balance ?? account?.pusd_balance ?? account?.usdc_balance ?? 0;
  const collateralSymbol = account?.collateral_symbol || 'pUSD';

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-slate-400 text-sm font-medium">正在加载持仓...</div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-black text-slate-900">持仓</h1>
        <p className="text-sm text-slate-400 font-medium mt-1">跟踪开仓和平仓</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        <SummaryCard icon="account_balance_wallet" label="组合市值" value={`$${(account?.portfolio_value || 0).toFixed(2)}`} />
        <SummaryCard icon="savings" label={`${collateralSymbol} 余额`} value={`$${collateralBalance.toFixed(2)}`} />
        <SummaryCard icon="swap_vert" label="持有中 / 已平仓" value={`${activeCount} / ${closedCount}`} />
        <SummaryCard icon="trending_up" label="总盈亏" value={`${totalPnl >= 0 ? '+' : ''}$${totalPnl.toFixed(2)}`} valueColor={totalPnl >= 0 ? 'text-secondary' : 'text-error'} />
      </div>

      {/* Positions table */}
      <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
        <div className="flex items-center gap-8 mb-6 border-b border-slate-100">
          {([
            { id: 'all' as const, label: '全部持仓' },
            { id: 'active' as const, label: '持有中' },
            { id: 'closed' as const, label: '已平仓' },
          ]).map(tab => (
            <button
              key={tab.id}
              onClick={() => setFilter(tab.id)}
              className={`pb-4 text-sm font-bold transition-colors ${
                filter === tab.id
                  ? 'font-black text-primary border-b-2 border-primary'
                  : 'text-slate-400 hover:text-slate-600'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {filtered.length === 0 ? (
          <div className="text-center py-12 text-slate-400 text-sm">暂无持仓数据</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead>
                <tr className="text-[10px] font-bold text-slate-400 uppercase tracking-widest">
                  <th className="pb-4 px-2">市场</th>
                  <th className="pb-4 px-2">方向</th>
                  <th className="pb-4 px-2">数量</th>
                  <th className="pb-4 px-2">开仓价</th>
                  <th className="pb-4 px-2">当前价</th>
                  <th className="pb-4 px-2">持仓时长</th>
                  <th className="pb-4 px-2">状态</th>
                  <th className="pb-4 px-2 text-right">盈亏</th>
                </tr>
              </thead>
              <tbody className="text-sm">
                {filtered.map((p, i) => (
                  <tr key={i} className="border-b border-slate-50 hover:bg-slate-50/50 transition-colors">
                    <td className="py-4 px-2 font-bold text-slate-900">{p.market_name || p.market_id}</td>
                    <td className="py-4 px-2">
                      <span className={`px-2 py-1 text-[10px] font-bold rounded-lg ${
                        isPositiveOutcome(p.side)
                          ? 'bg-secondary/10 text-secondary'
                          : 'bg-error/10 text-error'
                      }`}>
                        {p.side.toUpperCase()}
                      </span>
                    </td>
                    <td className="py-4 px-2 font-medium">{p.size.toFixed(2)}</td>
                    <td className="py-4 px-2 text-slate-500">${p.entry_price.toFixed(2)}</td>
                    <td className="py-4 px-2 text-slate-500">${p.current_price.toFixed(2)}</td>
                    <td className="py-4 px-2 text-slate-500 text-xs">{p.duration || '--'}</td>
                    <td className="py-4 px-2">
                      <span className={`px-2 py-1 text-[10px] font-bold rounded-lg ${
                        p.is_active ? 'bg-blue-100 text-blue-600' : 'bg-slate-100 text-slate-500'
                      }`}>
                        {p.is_active ? '持有中' : p.close_reason || '已平仓'}
                      </span>
                    </td>
                    <td className={`py-4 px-2 text-right font-black ${p.pnl >= 0 ? 'text-secondary' : 'text-error'}`}>
                      {p.pnl >= 0 ? '+' : ''}${p.pnl.toFixed(2)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

function SummaryCard({ icon, label, value, valueColor }: {
  icon: string; label: string; value: string; valueColor?: string;
}) {
  return (
    <div className="bg-surface-container-lowest p-5 rounded-[2rem] neo-shadow border border-white/50">
      <div className="flex items-center gap-2 mb-2">
        <MIcon name={icon} className="text-primary text-lg" filled />
        <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider">{label}</span>
      </div>
      <p className={`text-xl font-black ${valueColor || 'text-slate-900'}`}>{value}</p>
    </div>
  );
}
