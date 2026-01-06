import { useCallback, useEffect, useState } from 'react';
import clsx from 'clsx';
import {
  PieChart,
  Pie,
  Cell,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  LineChart,
  Line,
  Legend
} from 'recharts';
import { fetchStats } from '../lib/api';
import type { StatsResponse, BatchDTO, EvaluationDTO, BatchSummary } from '../types';
import ScoreBadge from './ScoreBadge';

interface DashboardProps {
  selectedBatch: BatchDTO | null;
  onSelectBatch: (batch: BatchDTO) => void;
}

// Color palette matching existing design
const COLORS = {
  block: '#8f2f1d',
  review: '#1e4fbf',
  caution: '#8a5a00',
  allow: '#1f6b3d',
  blockBg: '#ffe9e4',
  reviewBg: '#e7f0ff',
  cautionBg: '#fff4da',
  allowBg: '#e6f7ee'
};

const PIE_COLORS = [COLORS.block, COLORS.review, COLORS.caution, COLORS.allow];

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

interface RecommendationPieChartProps {
  data: {
    block: number;
    review: number;
    allow_with_caution: number;
    allow: number;
  };
}

function RecommendationPieChart({ data }: RecommendationPieChartProps) {
  const chartData = [
    { name: 'Block', value: data.block, color: COLORS.block },
    { name: 'Review', value: data.review, color: COLORS.review },
    { name: 'Caution', value: data.allow_with_caution, color: COLORS.caution },
    { name: 'Allow', value: data.allow, color: COLORS.allow }
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

interface ScoreDistributionChartProps {
  trademarkData: { score: number; count: number }[];
  viceData: { score: number; count: number }[];
}

function ScoreDistributionChart({ trademarkData, viceData }: ScoreDistributionChartProps) {
  // Ensure all scores 0-5 are represented
  const fillScores = (data: { score: number; count: number }[]) => {
    const map = new Map(data.map((d) => [d.score, d.count]));
    return Array.from({ length: 6 }, (_, i) => ({
      score: i,
      count: map.get(i) ?? 0
    }));
  };

  const combinedData = Array.from({ length: 6 }, (_, i) => ({
    score: i.toString(),
    trademark: fillScores(trademarkData).find((d) => d.score === i)?.count ?? 0,
    vice: fillScores(viceData).find((d) => d.score === i)?.count ?? 0
  }));

  return (
    <ResponsiveContainer width="100%" height={200}>
      <BarChart data={combinedData} margin={{ top: 10, right: 10, left: -10, bottom: 0 }}>
        <XAxis
          dataKey="score"
          tick={{ fontSize: 11, fill: 'var(--muted)' }}
          axisLine={{ stroke: 'var(--line)' }}
          tickLine={false}
        />
        <YAxis
          tick={{ fontSize: 11, fill: 'var(--muted)' }}
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
          formatter={(value: number) => formatNumber(value)}
        />
        <Legend
          wrapperStyle={{ fontSize: '11px' }}
          formatter={(value) => (value === 'trademark' ? 'Trademark' : 'Vice')}
        />
        <Bar dataKey="trademark" fill={COLORS.review} radius={[4, 4, 0, 0]} />
        <Bar dataKey="vice" fill={COLORS.block} radius={[4, 4, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  );
}

interface TopRisksPanelProps {
  title: string;
  items: EvaluationDTO[];
  scoreKey: 'trademark_score' | 'vice_score';
}

function TopRisksPanel({ title, items, scoreKey }: TopRisksPanelProps) {
  if (items.length === 0) {
    return (
      <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
        <h3 className="font-semibold text-[var(--text)] mb-3">{title}</h3>
        <div className="text-sm text-[var(--muted)]">No high-risk domains found</div>
      </section>
    );
  }

  return (
    <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
      <h3 className="font-semibold text-[var(--text)] mb-3">{title}</h3>
      <div className="space-y-3">
        {items.map((item) => (
          <div
            key={item.id}
            className="flex items-center justify-between gap-3 py-2 border-b border-[var(--line)] last:border-0"
          >
            <div className="flex-1 min-w-0">
              <div className="text-sm font-medium text-[var(--text)] truncate">
                {item.domain}
              </div>
              <div className="text-xs text-[var(--muted)] truncate">
                {scoreKey === 'trademark_score' && item.matched_trademark
                  ? `Matched: ${item.matched_trademark}`
                  : item.vice_categories?.join(', ') || 'No categories'}
              </div>
            </div>
            <ScoreBadge
              score={item[scoreKey]}
              type={scoreKey === 'trademark_score' ? 'trademark' : 'vice'}
            />
          </div>
        ))}
      </div>
    </section>
  );
}

interface BatchOverviewTableProps {
  batches: BatchSummary[];
  onSelectBatch: (batch: BatchDTO) => void;
  selectedBatchId?: number;
}

function BatchOverviewTable({
  batches,
  onSelectBatch,
  selectedBatchId
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
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[#8f2f1d]">
              Block
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[#1e4fbf]">
              Review
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[#8a5a00]">
              Caution
            </th>
            <th className="text-right py-2 px-2 text-[10px] font-semibold uppercase tracking-[0.15em] text-[#1f6b3d]">
              Allow
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

            return (
              <tr
                key={batch.id}
                onClick={() =>
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
                  })
                }
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
                <td className="py-2 px-2 text-right text-[#8f2f1d] font-medium">
                  {formatNumber(batch.block_count)}
                </td>
                <td className="py-2 px-2 text-right text-[#1e4fbf] font-medium">
                  {formatNumber(batch.review_count)}
                </td>
                <td className="py-2 px-2 text-right text-[#8a5a00] font-medium">
                  {formatNumber(batch.caution_count)}
                </td>
                <td className="py-2 px-2 text-right text-[#1f6b3d] font-medium">
                  {formatNumber(batch.allow_count)}
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

export default function Dashboard({ selectedBatch, onSelectBatch }: DashboardProps) {
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
        <StatCard
          label="Avg Processing"
          value={`${stats.avg_processing_time_ms.toFixed(0)}ms`}
        />
        <StatCard
          label="Block"
          value={stats.recommendation_counts.block}
          accent="bg-[#ffe9e4]"
        />
        <StatCard
          label="Review"
          value={stats.recommendation_counts.review}
          accent="bg-[#e7f0ff]"
        />
      </div>

      {/* Charts Row */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Recommendation Distribution Pie Chart */}
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">
            Risk Distribution
          </h3>
          <RecommendationPieChart data={stats.recommendation_counts} />
        </section>

        {/* Score Distribution Bar Charts */}
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">
            Score Distribution
          </h3>
          <ScoreDistributionChart
            trademarkData={stats.trademark_distribution ?? []}
            viceData={stats.vice_distribution ?? []}
          />
        </section>
      </div>

      {/* Top Risks Row */}
      <div className="grid gap-6 lg:grid-cols-2">
        <TopRisksPanel
          title="Top Trademark Risks"
          items={stats.top_trademark_risks ?? []}
          scoreKey="trademark_score"
        />
        <TopRisksPanel
          title="Top Vice Risks"
          items={stats.top_vice_risks ?? []}
          scoreKey="vice_score"
        />
      </div>

      {/* Batch Overview */}
      {stats.recent_batches && stats.recent_batches.length > 0 && (
        <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
          <h3 className="font-semibold text-[var(--text)] mb-4">Batch Overview</h3>
          <BatchOverviewTable
            batches={stats.recent_batches}
            onSelectBatch={onSelectBatch}
            selectedBatchId={selectedBatch?.id}
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
