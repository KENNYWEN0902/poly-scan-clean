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

function asNumber(value: unknown, fallback = 0) {
  const num = Number(value);
  return Number.isFinite(num) ? num : fallback;
}

function formatMoney(value: unknown) {
  const num = asNumber(value);
  return `${num >= 0 ? '' : '-'}$${Math.abs(num).toFixed(2)}`;
}

function formatSignedMoney(value: unknown) {
  const num = asNumber(value);
  return `${num >= 0 ? '+' : '-'}$${Math.abs(num).toFixed(2)}`;
}

function formatDate(value: unknown) {
  const date = new Date(String(value || ''));
  return Number.isNaN(date.getTime()) ? '--' : date.toLocaleString();
}

function formatDateOnly(value: unknown) {
  const date = new Date(String(value || ''));
  return Number.isNaN(date.getTime()) ? '--' : date.toLocaleDateString();
}

function tradeStatusLabel(status?: string) {
  const normalized = (status || '').toLowerCase();
  switch (normalized) {
    case 'filled': return '已成交';
    case 'failed': return '失败';
    case 'pending': return '处理中';
    case 'active': return '持仓中';
    case 'closed': return '已关闭';
    case 'won': return '盈利';
    case 'lost': return '亏损';
    default: return status ? status.toUpperCase() : '--';
  }
}

export default function PnlHistory() {
  const [trades, setTrades] = useState<TradeInfo[]>([]);
  const [perf, setPerf] = useState<PerformanceStats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    api.getTrades().catch(() => []).then(t => {
      if (cancelled) return;
      setTrades(Array.isArray(t) ? t : []);
      setLoading(false);
    });

    api.getPerformance().then(p => {
      if (!cancelled && p) setPerf(p);
    }).catch(() => {});

    return () => {
      cancelled = true;
    };
  }, []);

  const cumulativePnl = useMemo(() => {
    let cum = 0;
    return trades.map(t => {
      const pnl = asNumber(t.pnl);
      cum += pnl;
      return {
        time: formatDateOnly(t.timestamp),
        pnl,
        cumulative: cum,
      };
    });
  }, [trades]);

  const totalPnl = asNumber(perf?.total_pnl ?? trades.reduce((s, t) => s + asNumber(t.pnl), 0));
  const winCount = trades.filter(t => asNumber(t.pnl) > 0).length;
  const lossCount = trades.filter(t => asNumber(t.pnl) < 0).length;

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-slate-400 text-sm font-medium">正在加载历史...</div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-black text-slate-900">交易历史</h1>
        <p className="text-sm text-slate-400 font-medium mt-1">完整交易记录与盈亏分析</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        <SummaryCard icon="payments" label="总盈亏" value={formatSignedMoney(totalPnl)} valueColor={totalPnl >= 0 ? 'text-secondary' : 'text-error'} />
        <SummaryCard icon="check_circle" label="盈利笔数" value={String(winCount)} valueColor="text-secondary" />
        <SummaryCard icon="cancel" label="亏损笔数" value={String(lossCount)} valueColor="text-error" />
        <SummaryCard icon="tag" label="总交易数" value={String(trades.length)} />
      </div>

      {/* Cumulative P&L chart */}
      {cumulativePnl.length > 0 && (
        <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
          <h2 className="text-xl font-black text-slate-900 mb-2">累计盈亏</h2>
          <p className="text-sm text-slate-400 font-medium mb-6">随时间变化的累计盈亏</p>
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
          <h2 className="text-xl font-black text-slate-900 mb-6">交易日志</h2>
        {trades.length === 0 ? (
          <div className="text-center py-12 text-slate-400 text-sm">暂无交易记录</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left">
              <thead>
                <tr className="text-[10px] font-bold text-slate-400 uppercase tracking-widest">
                  <th className="pb-4 px-2">时间</th>
                  <th className="pb-4 px-2">市场</th>
                  <th className="pb-4 px-2">方向</th>
                  <th className="pb-4 px-2">价格</th>
                  <th className="pb-4 px-2">金额</th>
                  <th className="pb-4 px-2">置信度</th>
                  <th className="pb-4 px-2">状态</th>
                  <th className="pb-4 px-2 text-right">盈亏</th>
                </tr>
              </thead>
              <tbody className="text-sm">
                {trades.map((t, i) => (
                  <tr key={i} className="border-b border-slate-50 hover:bg-slate-50/50 transition-colors">
                    <td className="py-4 px-2 text-slate-500 text-xs">{formatDate(t.timestamp)}</td>
                    <td className="py-4 px-2 font-bold text-slate-900">{t.market_name || t.market_id}</td>
                    <td className="py-4 px-2">
                      <span className={`px-2 py-1 text-[10px] font-bold rounded-lg ${
                        t.direction === 'UP' || t.side?.toLowerCase() === 'buy'
                          ? 'bg-secondary/10 text-secondary'
                          : 'bg-error/10 text-error'
                      }`}>
                        {t.direction || t.side}
                      </span>
                    </td>
                    <td className="py-4 px-2 text-slate-500">{formatMoney(t.price)}</td>
                    <td className="py-4 px-2 font-medium">{formatMoney(t.total)}</td>
                    <td className="py-4 px-2 text-slate-500">{(asNumber(t.confidence) * 100).toFixed(0)}%</td>
                    <td className="py-4 px-2">
                      <span className={`px-2 py-1 text-[10px] font-bold rounded-lg ${
                        t.status === 'filled' ? 'bg-secondary/10 text-secondary'
                          : t.status === 'failed' ? 'bg-error/10 text-error'
                          : 'bg-blue-100 text-blue-600'
                      }`}>
                        {tradeStatusLabel(t.status)}
                      </span>
                    </td>
                    <td className={`py-4 px-2 text-right font-black ${asNumber(t.pnl) >= 0 ? 'text-secondary' : 'text-error'}`}>
                      {formatSignedMoney(t.pnl)}
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
