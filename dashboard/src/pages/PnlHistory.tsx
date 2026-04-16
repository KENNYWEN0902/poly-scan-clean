import { useState, useEffect, useMemo } from 'react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts';
import { api } from '../api';
import { TradeInfo, PerformanceStats } from '../types';

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

export default function PnlHistory() {
  const [trades, setTrades] = useState<TradeInfo[]>([]);
  const [perf, setPerf] = useState<PerformanceStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      api.getTrades().catch(() => []),
      api.getPerformance().catch(() => null),
    ]).then(([t, p]) => {
      setTrades(t);
      if (p) setPerf(p);
      setLoading(false);
    });
  }, []);

  const cumulativePnl = useMemo(() => {
    let cum = 0;
    return trades.map(t => {
      cum += t.pnl;
      return {
        time: new Date(t.timestamp).toLocaleDateString(),
        pnl: t.pnl,
        cumulative: cum,
      };
    });
  }, [trades]);

  const totalPnl = perf?.total_pnl ?? trades.reduce((s, t) => s + t.pnl, 0);
  const winCount = trades.filter(t => t.pnl > 0).length;
  const lossCount = trades.filter(t => t.pnl < 0).length;

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-slate-400 text-sm font-medium">Loading history...</div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-black text-slate-900">Trade History</h1>
        <p className="text-sm text-slate-400 font-medium mt-1">Complete trade log and P&L analysis</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        <SummaryCard icon="payments" label="Total P&L" value={`${totalPnl >= 0 ? '+' : ''}$${totalPnl.toFixed(2)}`} valueColor={totalPnl >= 0 ? 'text-secondary' : 'text-error'} />
        <SummaryCard icon="check_circle" label="Wins" value={String(winCount)} valueColor="text-secondary" />
        <SummaryCard icon="cancel" label="Losses" value={String(lossCount)} valueColor="text-error" />
        <SummaryCard icon="tag" label="Total Trades" value={String(trades.length)} />
      </div>

      {/* Cumulative P&L chart */}
      {cumulativePnl.length > 0 && (
        <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
          <h2 className="text-xl font-black text-slate-900 mb-2">Cumulative P&L</h2>
          <p className="text-sm text-slate-400 font-medium mb-6">Running total profit and loss over time</p>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={cumulativePnl} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
                <defs>
                  <linearGradient id="cumGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#006c49" stopOpacity={0.2} />
                    <stop offset="100%" stopColor="#006c49" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="#eaeef2" />
                <XAxis dataKey="time" tick={{ fontSize: 10, fill: '#94a3b8' }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fontSize: 10, fill: '#94a3b8' }} axisLine={false} tickLine={false} width={50} />
                <Tooltip
                  contentStyle={{
                    background: 'rgba(255,255,255,0.9)',
                    border: '1px solid #eaeef2',
                    borderRadius: '12px',
                    fontSize: '12px',
                    fontWeight: 600,
                  }}
                />
                <Area type="monotone" dataKey="cumulative" stroke="#006c49" strokeWidth={3} fill="url(#cumGrad)" dot={false} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Trade log table */}
      <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
        <h2 className="text-xl font-black text-slate-900 mb-6">Trade Log</h2>
        {trades.length === 0 ? (
          <div className="text-center py-12 text-slate-400 text-sm">No trades recorded</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead>
                <tr className="text-[10px] font-bold text-slate-400 uppercase tracking-widest">
                  <th className="pb-4 px-2">Time</th>
                  <th className="pb-4 px-2">Market</th>
                  <th className="pb-4 px-2">Direction</th>
                  <th className="pb-4 px-2">Price</th>
                  <th className="pb-4 px-2">Size</th>
                  <th className="pb-4 px-2">Confidence</th>
                  <th className="pb-4 px-2">Status</th>
                  <th className="pb-4 px-2 text-right">P&L</th>
                </tr>
              </thead>
              <tbody className="text-sm">
                {trades.map((t, i) => (
                  <tr key={i} className="border-b border-slate-50 hover:bg-slate-50/50 transition-colors">
                    <td className="py-4 px-2 text-slate-500 text-xs">{new Date(t.timestamp).toLocaleString()}</td>
                    <td className="py-4 px-2 font-bold text-slate-900">{t.market_name || t.market_id}</td>
                    <td className="py-4 px-2">
                      <span className={`px-2 py-1 text-[10px] font-bold rounded-lg ${
                        t.direction === 'UP' || t.side === 'buy'
                          ? 'bg-secondary/10 text-secondary'
                          : 'bg-error/10 text-error'
                      }`}>
                        {t.direction || t.side}
                      </span>
                    </td>
                    <td className="py-4 px-2 text-slate-500">${t.price.toFixed(2)}</td>
                    <td className="py-4 px-2 font-medium">${t.total.toFixed(2)}</td>
                    <td className="py-4 px-2 text-slate-500">{(t.confidence * 100).toFixed(0)}%</td>
                    <td className="py-4 px-2">
                      <span className={`px-2 py-1 text-[10px] font-bold rounded-lg ${
                        t.status === 'filled' ? 'bg-secondary/10 text-secondary'
                          : t.status === 'failed' ? 'bg-error/10 text-error'
                          : 'bg-blue-100 text-blue-600'
                      }`}>
                        {t.status.toUpperCase()}
                      </span>
                    </td>
                    <td className={`py-4 px-2 text-right font-black ${t.pnl >= 0 ? 'text-secondary' : 'text-error'}`}>
                      {t.pnl >= 0 ? '+' : ''}${t.pnl.toFixed(2)}
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
