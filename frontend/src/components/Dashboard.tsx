import { useCallback, useEffect, useState } from 'react';
import clsx from 'clsx';
import {
  PieChart,
  Pie,
  Cell,
  Tooltip,
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis
} from 'recharts';
import { fetchStats } from '../lib/api';
import type { StatsResponse, BatchDTO, BatchSummary } from '../types';
import ScoreBadge from './ScoreBadge';

interface DashboardProps {
  selectedBatch: BatchDTO | null;
  onSelectBatch: (batch: BatchDTO) => void;
  onSwitchToResults?: () => void;
}

// Color palette for 3-tier system
const COLORS = {
  yes_risk: '#dc2626',      // red-600
  potential_risk: '#d97706', // amber-600
  no_risk: '#16a34a',       // green-600
  yes_riskBg: '#fef2f2',    // red-50
  potential_riskBg: '#fffbeb', // amber-50
  no_riskBg: '#f0fdf4'      // green-50
};

const PIE_COLORS = [COLORS.yes_risk, COLORS.potential_risk, COLORS.no_risk];

function formatNumber(num: number): string {
  if (num >= 1000000) {
    return `${(num / 1000000).toFixed(1)}M`;
  }
  if (num >= 1000) {
    return `${(num / 1000).toFixed(1)}K`;
  }
  return num.toLocaleString();
}

interface StatCardProps {
  label: string;
  value: string | number;
  accent?: string;
}

function StatCard({ label, value, accent }: StatCardProps) {
  return (
    <div
      className={clsx(
        'rounded-xl border border-[var(--line)] p-4 flex flex-col gap-1',
        accent ?? 'bg-[var(--surface)]'
      )}
    >
      <span className="text-sm font-medium text-[var(--muted)]">
        {label}
      </span>
      <span className="text-2xl font-bold text-[var(--text)]">
        {typeof value === 'number' ? formatNumber(value) : value}
      </span>
    </div>
  );
}

interface RiskDistributionChartProps {
  data: {
    yes_risk: number;
    potential_risk: number;
    no_risk: number;
  };
}

function RiskDistributionChart({ data }: RiskDistributionChartProps) {
  const chartData = [
    { name: 'Yes Risk', value: data.yes_risk, color: COLORS.yes_risk },
    { name: 'Potential Risk', value: data.potential_risk, color: COLORS.potential_risk },
    { name: 'No Risk', value: data.no_risk, color: COLORS.no_risk }
  ].filter((d) => d.value > 0);

  const total = chartData.reduce((sum, d) => sum + d.value, 0);

  if (total === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-[var(--muted)]">
        No data available
      </div>
    );
  }

  return (
    <div className="flex items-center gap-6">
      <ResponsiveContainer width={180} height={180}>
        <PieChart>
          <Pie
            data={chartData}
            cx="50%"
            cy="50%"
            innerRadius={45}
            outerRadius={80}
            paddingAngle={2}
            dataKey="value"
          >
            {chartData.map((entry, index) => (
              <Cell key={`cell-${index}`} fill={entry.color} />
            ))}
          </Pie>
          <Tooltip
            formatter={(value: number) => [formatNumber(value), 'Count']}
            contentStyle={{
              backgroundColor: 'var(--surface)',
              border: '1px solid var(--line)',
              borderRadius: '8px',
              fontSize: '12px'
            }}
          />
        </PieChart>
      </ResponsiveContainer>
      <div className="flex flex-col gap-2">
        {chartData.map((entry) => (
          <div key={entry.name} className="flex items-center gap-2">
            <div
              className="w-3 h-3 rounded-full"
              style={{ backgroundColor: entry.color }}
            />
            <span className="text-xs text-[var(--muted)]">{entry.name}</span>
            <span className="text-xs font-semibold text-[var(--text)]">
              {formatNumber(entry.value)} ({((entry.value / total) * 100).toFixed(1)}%)
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

interface BatchOverviewTableProps {
  batches: BatchSummary[];
  onSelectBatch: (batch: BatchDTO) => void;
  selectedBatchId?: number;
  onSwitchToResults?: () => void;
}

function BatchOverviewTable({
  batches,
  onSelectBatch,
  selectedBatchId,
  onSwitchToResults
}: BatchOverviewTableProps) {
  if (batches.length === 0) {
    return <div className="text-sm text-[var(--muted)]">No batches available</div>;
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--line)]">
            <th className="text-left py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[var(--muted)]">
              Batch
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[var(--muted)]">
              Domains
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[var(--muted)]">
              Progress
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[#dc2626]">
              TM Risk
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[#d97706]">
              Vice Risk
            </th>
          </tr>
        </thead>
        <tbody>
          {batches.map((batch) => {
            const progress =
              batch.total_domains > 0
                ? Math.round((batch.processed_domains / batch.total_domains) * 100)
                : 0;
            const isSelected = batch.id === selectedBatchId;

            // Calculate combined trademark and vice risks
            const tmRisks = (batch.trademark_yes_risk ?? 0) + (batch.trademark_potential_risk ?? 0);
            const viceRisks = (batch.vice_yes_risk ?? 0) + (batch.vice_potential_risk ?? 0);

            return (
              <tr
                key={batch.id}
                onClick={() => {
                  onSelectBatch({
                    id: batch.id,
                    name: batch.name,
                    owner: batch.owner,
                    original_filename: '',
                    row_count: batch.total_domains,
                    unique_domains: batch.total_domains,
                    existing_domains: 0,
                    duplicate_rows: 0,
                    processed_domains: batch.processed_domains,
                    created_at: batch.created_at,
                    last_evaluated_at: batch.last_evaluated_at
                  });
                  onSwitchToResults?.();
                }}
                className={clsx(
                  'border-b border-[var(--line)] cursor-pointer transition-colors hover:bg-[var(--surface-2)]',
                  isSelected && 'bg-[var(--accent-soft)]'
                )}
              >
                <td className="py-2 px-2 font-medium text-[var(--text)]">{batch.name}</td>
                <td className="py-2 px-2 text-right text-[var(--muted)]">
                  {formatNumber(batch.total_domains)}
                </td>
                <td className="py-2 px-2 text-right">
                  <div className="flex items-center justify-end gap-2">
                    <div className="w-16 h-1.5 bg-[var(--line)] rounded-full overflow-hidden">
                      <div
                        className="h-full bg-[var(--accent)] rounded-full"
                        style={{ width: `${progress}%` }}
                      />
                    </div>
                    <span className="text-xs text-[var(--muted)]">{progress}%</span>
                  </div>
                </td>
                <td className="py-2 px-2 text-right text-[#dc2626] font-medium">
                  {formatNumber(tmRisks)}
                </td>
                <td className="py-2 px-2 text-right text-[#d97706] font-medium">
                  {formatNumber(viceRisks)}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

interface ActivitySparklineProps {
  data: { date: string; count: number }[];
}

function ActivitySparkline({ data }: ActivitySparklineProps) {
  if (data.length === 0) {
    return (
      <div className="flex items-center justify-center h-24 text-[var(--muted)]">
        No activity data
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height={120}>
      <LineChart data={data} margin={{ top: 10, right: 10, left: -10, bottom: 0 }}>
        <XAxis
          dataKey="date"
          tick={{ fontSize: 10, fill: 'var(--muted)' }}
          axisLine={{ stroke: 'var(--line)' }}
          tickLine={false}
          tickFormatter={(value) => {
            const date = new Date(value);
            return `${date.getMonth() + 1}/${date.getDate()}`;
          }}
        />
        <YAxis
          tick={{ fontSize: 10, fill: 'var(--muted)' }}
          axisLine={false}
          tickLine={false}
          tickFormatter={formatNumber}
        />
        <Tooltip
          contentStyle={{
            backgroundColor: 'var(--surface)',
            border: '1px solid var(--line)',
            borderRadius: '8px',
            fontSize: '12px'
          }}
          labelFormatter={(label) => {
            const date = new Date(label);
            return date.toLocaleDateString();
          }}
          formatter={(value: number) => [formatNumber(value), 'Evaluations']}
        />
        <Line
          type="monotone"
          dataKey="count"
          stroke="var(--accent)"
          strokeWidth={2}
          dot={false}
          activeDot={{ r: 4, fill: 'var(--accent)' }}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-20 rounded-xl bg-[var(--surface-2)]" />
        ))}
      </div>
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="h-64 rounded-xl bg-[var(--surface-2)]" />
        <div className="h-64 rounded-xl bg-[var(--surface-2)]" />
      </div>
    </div>
  );
}

function DashboardError({
  message,
  onRetry
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div className="flex flex-col items-center justify-center py-12 gap-4">
      <div className="text-[var(--muted)]">{message}</div>
      <button
        type="button"
        onClick={onRetry}
        className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm font-medium text-[var(--text)] hover:bg-[var(--surface-2)]"
      >
        Retry
      </button>
    </div>
  );
}

export default function Dashboard({ selectedBatch, onSelectBatch, onSwitchToResults }: DashboardProps) {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadStats = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchStats(selectedBatch?.id);
      setStats(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load statistics');
    } finally {
      setLoading(false);
    }
  }, [selectedBatch?.id]);

  useEffect(() => {
    loadStats();
  }, [loadStats]);

  if (loading) {
    return <DashboardSkeleton />;
  }

  if (error) {
    return <DashboardError message={error} onRetry={loadStats} />;
  }

  if (!stats) {
    return null;
  }

  // Separate Trademark and Vice recommendation counts
  const trademarkCounts = {
    yes_risk: stats.trademark_recommendation_counts?.yes_risk ?? 0,
    potential_risk: stats.trademark_recommendation_counts?.potential_risk ?? 0,
    no_risk: stats.trademark_recommendation_counts?.no_risk ?? 0
  };

  const viceCounts = {
    yes_risk: stats.vice_recommendation_counts?.yes_risk ?? 0,
    potential_risk: stats.vice_recommendation_counts?.potential_risk ?? 0,
    no_risk: stats.vice_recommendation_counts?.no_risk ?? 0
  };

  // Calculate totals for the stat cards
  const totalTrademarkRisks = trademarkCounts.yes_risk + trademarkCounts.potential_risk;
  const totalViceRisks = viceCounts.yes_risk + viceCounts.potential_risk;

  return (
    <div className="space-y-6">
      {/* Header */}
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-[var(--text)]">Dashboard</h2>
          <p className="text-sm text-[var(--muted)]">
            {selectedBatch ? `${selectedBatch.name} overview` : 'Global statistics'}
          </p>
        </div>
        <button
          type="button"
          onClick={loadStats}
          className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm font-medium text-[var(--text)] hover:bg-[var(--surface-2)] transition-colors"
        >
          Refresh
        </button>
      </header>

      {/* Stat Cards Row */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Total Evaluated" value={stats.total_evaluations} />
        <StatCard label="Total Batches" value={stats.total_batches} />
        <StatCard
          label="Trademark Risks"
          value={totalTrademarkRisks}
          accent="bg-red-50"
        />
        <StatCard
          label="Vice Risks"
          value={totalViceRisks}
          accent="bg-amber-50"
        />
      </div>

      {/* Risk Distribution Charts - Two separate pie charts */}
      <div className="grid gap-6 lg:grid-cols-2">
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">
            Trademark Risk Distribution
          </h3>
          <RiskDistributionChart data={trademarkCounts} />
        </section>
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">
            Vice Risk Distribution
          </h3>
          <RiskDistributionChart data={viceCounts} />
        </section>
      </div>

      {/* Batch Overview */}
      {stats.recent_batches && stats.recent_batches.length > 0 && (
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">Batch Overview</h3>
          <BatchOverviewTable
            batches={stats.recent_batches}
            onSelectBatch={onSelectBatch}
            selectedBatchId={selectedBatch?.id}
            onSwitchToResults={onSwitchToResults}
          />
        </section>
      )}

      {/* Activity Sparkline */}
      {stats.evaluations_over_time && stats.evaluations_over_time.length > 0 && (
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">
            Evaluation Activity (Last 30 Days)
          </h3>
          <ActivitySparkline data={stats.evaluations_over_time} />
        </section>
      )}
    </div>
  );
}
