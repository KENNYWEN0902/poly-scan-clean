import { useState, useEffect } from 'react';
import { api } from '../api';
import { StrategyState, StrategyConfig, PerformanceStats } from '../types';

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

export default function Strategies() {
  const [strategy, setStrategy] = useState<StrategyState | null>(null);
  const [config, setConfig] = useState<StrategyConfig | null>(null);
  const [perf, setPerf] = useState<PerformanceStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      api.getStrategy().catch(() => null),
      api.getConfig().catch(() => null),
      api.getPerformance().catch(() => null),
    ]).then(([s, c, p]) => {
      if (s) setStrategy(s);
      if (c) setConfig(c);
      if (p) setPerf(p);
      setLoading(false);
    });
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-slate-400 text-sm font-medium">Loading strategies...</div>
      </div>
    );
  }

  const winRate = perf?.win_rate ?? strategy?.win_rate ?? 0;
  const totalTrades = perf?.total_trades ?? strategy?.total_trades ?? 0;
  const totalPnl = perf?.total_pnl ?? strategy?.total_pnl ?? 0;

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-black text-slate-900">Strategies</h1>
        <p className="text-sm text-slate-400 font-medium mt-1">Manage and monitor trading strategies</p>
      </div>

      {/* Strategy card */}
      <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
        <div className="flex items-start justify-between mb-6">
          <div className="flex items-center gap-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-primary to-primary-container flex items-center justify-center text-white shadow-lg shadow-primary/20">
              <MIcon name="bolt" filled className="text-2xl" />
            </div>
            <div>
              <h2 className="text-xl font-black text-slate-900">{strategy?.name || 'BTC 5-Min Delay Arb'}</h2>
              <p className="text-sm text-slate-400">Polymarket BTC prediction markets</p>
            </div>
          </div>
          <span className={`px-4 py-2 rounded-xl text-xs font-black uppercase tracking-wider ${
            strategy?.status === 'running'
              ? 'bg-secondary/10 text-secondary'
              : 'bg-slate-100 text-slate-500'
          }`}>
            {strategy?.status || 'Inactive'}
          </span>
        </div>

        {/* Stats grid */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-6 mt-8">
          <StatBox icon="trending_up" label="Total P&L" value={`${totalPnl >= 0 ? '+' : ''}$${totalPnl.toFixed(2)}`} color={totalPnl >= 0 ? 'text-secondary' : 'text-error'} />
          <StatBox icon="speed" label="Win Rate" value={`${winRate.toFixed(1)}%`} color="text-primary" />
          <StatBox icon="tag" label="Total Trades" value={String(totalTrades)} color="text-slate-900" />
          <StatBox icon="schedule" label="Uptime" value={strategy?.uptime || perf?.uptime || '--'} color="text-slate-900" />
        </div>

        {/* Strategy parameters */}
        {config && (
          <div className="mt-8 pt-8 border-t border-slate-100">
            <h3 className="text-sm font-black text-slate-900 uppercase tracking-wider mb-4">Strategy Parameters</h3>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              <ParamItem label="Min Confidence" value={`${(config.min_confidence * 100).toFixed(0)}%`} />
              <ParamItem label="Max Position" value={`$${config.max_position_usd.toFixed(0)}`} />
              <ParamItem label="Min Price Change" value={`${(config.min_price_change * 100).toFixed(2)}%`} />
              <ParamItem label="Predict Before End" value={`${config.predict_before_end}s`} />
              <ParamItem label="Cooldown/Market" value={`${config.cooldown_per_market}s`} />
              <ParamItem label="Dynamic Pricing" value={config.use_dynamic_pricing ? 'Enabled' : 'Disabled'} />
            </div>
          </div>
        )}

        {/* Risk parameters */}
        {config?.enable_risk_mgmt && (
          <div className="mt-8 pt-8 border-t border-slate-100">
            <h3 className="text-sm font-black text-slate-900 uppercase tracking-wider mb-4">Risk Controls</h3>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
              <ParamItem label="Max Daily Loss" value={`$${config.max_daily_loss.toFixed(0)}`} />
              <ParamItem label="Max Drawdown" value={`${(config.max_drawdown_pct * 100).toFixed(0)}%`} />
              <ParamItem label="Max Consecutive Loss" value={String(config.max_consecutive_loss)} />
              <ParamItem label="Max Daily Trades" value={String(config.max_daily_trades)} />
              <ParamItem label="Price Slippage" value={`${(config.price_slippage * 100).toFixed(1)}%`} />
            </div>
          </div>
        )}
      </div>

      {/* Performance summary */}
      {perf && (
        <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
          <h2 className="text-xl font-black text-slate-900 mb-6">Performance Summary</h2>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
            <StatBox icon="emoji_events" label="Best Trade" value={`+$${perf.best_trade.toFixed(2)}`} color="text-secondary" />
            <StatBox icon="warning" label="Worst Trade" value={`$${perf.worst_trade.toFixed(2)}`} color="text-error" />
            <StatBox icon="analytics" label="Sharpe Ratio" value={perf.sharpe_ratio.toFixed(2)} color="text-primary" />
            <StatBox icon="timer" label="Avg Hold Time" value={perf.average_hold_time || '--'} color="text-slate-900" />
          </div>
        </div>
      )}
    </div>
  );
}

function StatBox({ icon, label, value, color }: { icon: string; label: string; value: string; color: string }) {
  return (
    <div className="p-4 bg-surface-container-low rounded-2xl">
      <div className="flex items-center gap-2 mb-2">
        <MIcon name={icon} className="text-slate-400 text-lg" />
        <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider">{label}</span>
      </div>
      <p className={`text-xl font-black ${color}`}>{value}</p>
    </div>
  );
}

function ParamItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="p-3 bg-surface-container-low rounded-xl">
      <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider block mb-1">{label}</span>
      <span className="text-sm font-bold text-slate-900">{value}</span>
    </div>
  );
}
