import { useState, useEffect, useCallback } from 'react';
import { api } from '../api';
import { StrategyConfig, AccountInfo } from '../types';

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

export default function SettingsPage() {
  const [config, setConfig] = useState<StrategyConfig | null>(null);
  const [account, setAccount] = useState<AccountInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [tradingStopped, setTradingStopped] = useState(false);
  const [toggling, setToggling] = useState(false);

  useEffect(() => {
    let cancelled = false;

    Promise.all([
      api.getConfig().catch(() => null),
      api.getTradingStopped().catch(() => false),
    ]).then(([c, stopped]) => {
      if (cancelled) return;
      if (c) setConfig(c);
      setTradingStopped(stopped);
      setLoading(false);
    });

    api.getAccount().then(a => {
      if (!cancelled && a) setAccount(a);
    }).catch(() => {});

    return () => {
      cancelled = true;
    };
  }, []);

  const handleToggleTrading = useCallback(async () => {
    setToggling(true);
    try {
      const newState = await api.setTradingStopped(!tradingStopped);
      setTradingStopped(newState);
    } catch { /* ignore */ }
    setToggling(false);
  }, [tradingStopped]);

  const handleSave = useCallback(async () => {
    if (!config) return;
    setSaving(true);
    try {
      const updated = await api.saveConfig(config);
      setConfig(updated);
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch { /* ignore */ }
    setSaving(false);
  }, [config]);

  const updateField = <K extends keyof StrategyConfig>(key: K, value: StrategyConfig[K]) => {
    if (!config) return;
    setConfig({ ...config, [key]: value });
  };

  const collateralBalance =
    account?.collateral_balance ?? account?.pusd_balance ?? account?.usdc_balance ?? 0;
  const collateralSymbol = account?.collateral_symbol || 'pUSD';

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-slate-400 text-sm font-medium">正在加载设置...</div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-black text-slate-900">设置</h1>
          <p className="text-sm text-slate-400 font-medium mt-1">配置策略与风控参数</p>
        </div>
        <button
          onClick={handleSave}
          disabled={saving || !config}
          className={`px-6 py-3 rounded-xl text-sm font-bold transition-all ${
            saved
              ? 'bg-secondary text-white'
              : 'bg-primary text-white hover:bg-primary-container shadow-lg shadow-primary/20'
          } disabled:opacity-50`}
        >
          {saved ? '已保存' : saving ? '保存中...' : '保存修改'}
        </button>
      </div>

      {/* Trading Control */}
      <div className={`p-8 rounded-[2.5rem] neo-shadow border ${tradingStopped ? 'bg-red-50 border-red-200' : 'bg-surface-container-lowest border-white/50'}`}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <MIcon name={tradingStopped ? 'pause_circle' : 'play_circle'} filled className={`text-3xl ${tradingStopped ? 'text-red-500' : 'text-secondary'}`} />
            <div>
              <h2 className="text-xl font-black text-slate-900">交易控制</h2>
              <p className="text-sm text-slate-400 font-medium mt-0.5">
                {tradingStopped ? '交易已暂停，系统不会再开新仓' : '机器人正在扫描并交易'}
              </p>
            </div>
          </div>
          <button
            onClick={handleToggleTrading}
            disabled={toggling}
            className={`px-8 py-3 rounded-xl text-sm font-bold transition-all shadow-lg ${
              tradingStopped
                ? 'bg-secondary text-white hover:bg-secondary/90 shadow-secondary/20'
                : 'bg-red-500 text-white hover:bg-red-600 shadow-red-500/20'
            } disabled:opacity-50`}
          >
            {toggling ? '...' : tradingStopped ? '▶ 恢复交易' : '⏸ 停止交易'}
          </button>
        </div>
      </div>

      {/* Account info */}
      {account && (
        <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
          <div className="flex items-center gap-3 mb-6">
            <MIcon name="account_circle" filled className="text-primary text-2xl" />
            <h2 className="text-xl font-black text-slate-900">账户</h2>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            <InfoItem label="钱包" value={account.wallet_address ? `${account.wallet_address.slice(0, 6)}...${account.wallet_address.slice(-4)}` : '--'} />
            <InfoItem label={`${collateralSymbol} 余额`} value={`$${collateralBalance.toFixed(2)}`} />
            <InfoItem label="组合市值" value={`$${account.portfolio_value.toFixed(2)}`} />
          </div>
        </div>
      )}

      {/* Strategy config */}
      {config && (
        <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
          <div className="flex items-center gap-3 mb-6">
            <MIcon name="tune" filled className="text-primary text-2xl" />
            <h2 className="text-xl font-black text-slate-900">策略配置</h2>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <NumberField label="最低置信度" value={config.min_confidence} onChange={v => updateField('min_confidence', v)} step={0.01} min={0} max={1} hint="0-1 范围" />
            <NumberField label="最小价格变动" value={config.min_price_change} onChange={v => updateField('min_price_change', v)} step={0.0001} min={0} hint="小数百分比" />
            <NumberField label="单笔最大仓位（USD）" value={config.max_position_usd} onChange={v => updateField('max_position_usd', v)} step={5} min={1} hint="每笔交易最高美元金额" />
            <NumberField label="提前预测时间（秒）" value={config.predict_before_end} onChange={v => updateField('predict_before_end', v)} step={1} min={1} hint="秒" />
            <NumberField label="提前执行时间（秒）" value={config.execution_lead_time} onChange={v => updateField('execution_lead_time', v)} step={1} min={0} hint="秒" />
            <NumberField label="单市场冷却时间（秒）" value={config.cooldown_per_market} onChange={v => updateField('cooldown_per_market', v)} step={1} min={0} hint="秒" />
            <NumberField label="价格滑点" value={config.price_slippage} onChange={v => updateField('price_slippage', v)} step={0.001} min={0} hint="小数" />
            <ToggleField label="动态定价" value={config.use_dynamic_pricing} onChange={v => updateField('use_dynamic_pricing', v)} />
          </div>
        </div>
      )}

      {/* Risk management */}
      {config && (
        <div className="bg-surface-container-lowest p-8 rounded-[2.5rem] neo-shadow border border-white/50">
          <div className="flex items-center gap-3 mb-6">
            <MIcon name="shield" filled className="text-primary text-2xl" />
            <h2 className="text-xl font-black text-slate-900">风险管理</h2>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <ToggleField label="启用风险管理" value={config.enable_risk_mgmt} onChange={v => updateField('enable_risk_mgmt', v)} />
            <NumberField label="单日最大亏损" value={config.max_daily_loss} onChange={v => updateField('max_daily_loss', v)} step={10} min={0} hint="美元" />
            <NumberField label="最大回撤 %" value={config.max_drawdown_pct} onChange={v => updateField('max_drawdown_pct', v)} step={0.01} min={0} max={1} hint="0-1 范围" />
            <NumberField label="最大连续亏损" value={config.max_consecutive_loss} onChange={v => updateField('max_consecutive_loss', v)} step={1} min={1} hint="笔数" />
            <NumberField label="单日最大交易数" value={config.max_daily_trades} onChange={v => updateField('max_daily_trades', v)} step={1} min={1} hint="笔数" />
          </div>
        </div>
      )}
    </div>
  );
}

function InfoItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="p-4 bg-surface-container-low rounded-2xl">
      <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider block mb-1">{label}</span>
      <span className="text-sm font-bold text-slate-900">{value}</span>
    </div>
  );
}

function NumberField({ label, value, onChange, step, min, max, hint }: {
  label: string; value: number; onChange: (v: number) => void;
  step?: number; min?: number; max?: number; hint?: string;
}) {
  return (
    <div>
      <label className="block text-xs font-bold text-slate-500 uppercase tracking-wider mb-2">{label}</label>
      <input
        type="number"
        value={value}
        onChange={e => onChange(parseFloat(e.target.value) || 0)}
        step={step}
        min={min}
        max={max}
        className="w-full bg-surface-container-low border-none rounded-xl px-4 py-3 text-sm font-medium text-slate-900 focus:ring-2 focus:ring-primary/20 outline-none transition-all"
      />
      {hint && <span className="text-[10px] text-slate-400 mt-1 block">{hint}</span>}
    </div>
  );
}

function ToggleField({ label, value, onChange }: {
  label: string; value: boolean; onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between p-4 bg-surface-container-low rounded-xl">
      <span className="text-xs font-bold text-slate-500 uppercase tracking-wider">{label}</span>
      <button
        onClick={() => onChange(!value)}
        className={`relative w-12 h-6 rounded-full transition-colors ${
          value ? 'bg-primary' : 'bg-slate-300'
        }`}
      >
        <span className={`absolute top-1 left-1 w-4 h-4 bg-white rounded-full transition-transform shadow ${
          value ? 'translate-x-6' : ''
        }`} />
      </button>
    </div>
  );
}
