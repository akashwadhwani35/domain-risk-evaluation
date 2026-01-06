import { useEffect, useState } from 'react';
import dayjs from 'dayjs';
import type { FeedbackStatsResponse, OverrideDTO } from '../types';
import { fetchFeedbackStats, fetchOverrides } from '../lib/api';

interface AILearningDashboardProps {
  batchId?: number;
}

export default function AILearningDashboard({ batchId }: AILearningDashboardProps) {
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState<FeedbackStatsResponse | null>(null);
  const [recentOverrides, setRecentOverrides] = useState<OverrideDTO[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function loadData() {
      setLoading(true);
      setError(null);
      try {
        const [statsData, overridesData] = await Promise.all([
          fetchFeedbackStats(batchId),
          fetchOverrides(0, 10)
        ]);
        setStats(statsData);
        setRecentOverrides(overridesData.items);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load AI learning data');
      } finally {
        setLoading(false);
      }
    }
    loadData();
  }, [batchId]);

  if (loading) {
    return (
      <div className="flex h-96 items-center justify-center">
        <div className="text-[var(--muted)]">Loading AI learning data...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-3xl border border-red-200 bg-red-50 p-8">
        <p className="text-red-600">{error}</p>
      </div>
    );
  }

  if (!stats) {
    return null;
  }

  return (
    <div className="space-y-8">
      {/* Overview Metrics */}
      <section className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          title="Total Overrides"
          value={stats.total_overrides.toLocaleString()}
          subtitle={`${stats.total_feedback_embeddings} feedback embeddings`}
        />
        <MetricCard
          title="Override Rate"
          value={`${stats.override_rate_percent.toFixed(1)}%`}
          subtitle="of evaluations corrected"
          trend={stats.override_rate_percent < 10 ? 'good' : stats.override_rate_percent < 25 ? 'neutral' : 'bad'}
        />
        <MetricCard
          title="AI Accuracy"
          value={`${stats.ai_accuracy.accuracy_percent.toFixed(1)}%`}
          subtitle="correct on first try"
          trend={stats.ai_accuracy.accuracy_percent > 90 ? 'good' : stats.ai_accuracy.accuracy_percent > 75 ? 'neutral' : 'bad'}
        />
        <MetricCard
          title="Feedback Impact"
          value={stats.feedback_impact_score.toFixed(2)}
          subtitle="avg retrievals per embedding"
        />
      </section>

      {/* Correction Patterns */}
      <section className="rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[0_16px_40px_rgba(18,18,18,0.06)]">
        <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Correction Patterns</h3>
        <p className="mb-4 text-sm text-[var(--muted)]">How often the AI is corrected from one recommendation to another</p>

        {stats.correction_patterns.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No corrections recorded yet.</p>
        ) : (
          <div className="space-y-3">
            {stats.correction_patterns.slice(0, 6).map((pattern, idx) => (
              <div key={idx} className="flex items-center gap-4">
                <div className="flex items-center gap-2 text-sm">
                  <RecommendationBadge recommendation={pattern.from_recommendation} />
                  <span className="text-[var(--muted)]">→</span>
                  <RecommendationBadge recommendation={pattern.to_recommendation} />
                </div>
                <div className="flex-1">
                  <div className="h-2 overflow-hidden rounded-full bg-[var(--surface-2)]">
                    <div
                      className="h-full rounded-full bg-purple-500"
                      style={{ width: `${Math.min(pattern.percentage, 100)}%` }}
                    />
                  </div>
                </div>
                <span className="min-w-[80px] text-right text-sm text-[var(--muted)]">
                  {pattern.count} ({pattern.percentage.toFixed(1)}%)
                </span>
              </div>
            ))}
          </div>
        )}

        <div className="mt-6 grid grid-cols-2 gap-4 border-t border-[var(--line)] pt-6">
          <div className="text-center">
            <div className="text-2xl font-bold text-[var(--text)]">{stats.ai_accuracy.block_to_allow_rate.toFixed(1)}%</div>
            <div className="text-xs text-[var(--muted)]">BLOCK → ALLOW corrections</div>
          </div>
          <div className="text-center">
            <div className="text-2xl font-bold text-[var(--text)]">{stats.ai_accuracy.allow_to_block_rate.toFixed(1)}%</div>
            <div className="text-xs text-[var(--muted)]">ALLOW → BLOCK corrections</div>
          </div>
        </div>
      </section>

      {/* Confidence Calibration */}
      <section className="rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[0_16px_40px_rgba(18,18,18,0.06)]">
        <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Confidence Calibration</h3>
        <p className="mb-4 text-sm text-[var(--muted)]">
          Accuracy per confidence level — ideally, high confidence should correlate with high accuracy
        </p>

        {stats.confidence_calibration.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No calibration data available.</p>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-5">
            {stats.confidence_calibration.map((bucket, idx) => (
              <div key={idx} className="rounded-2xl border border-[var(--line)] bg-[var(--surface-2)] p-4 text-center">
                <div className="text-xs text-[var(--muted)]">
                  {(bucket.confidence_min * 100).toFixed(0)}% - {(bucket.confidence_max * 100).toFixed(0)}%
                </div>
                <div className="mt-2 text-2xl font-bold text-[var(--text)]">
                  {bucket.accuracy_percent.toFixed(0)}%
                </div>
                <div className="text-xs text-[var(--muted)]">
                  {bucket.overridden_count}/{bucket.total_count} overridden
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Top Overriders */}
        <section className="rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[0_16px_40px_rgba(18,18,18,0.06)]">
          <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Top Overriders</h3>

          {stats.user_stats.length === 0 ? (
            <p className="text-sm text-[var(--muted)]">No user data available.</p>
          ) : (
            <div className="space-y-4">
              {stats.user_stats.map((user, idx) => (
                <div key={idx} className="flex items-center justify-between rounded-xl border border-[var(--line)] bg-[var(--surface-2)] p-3">
                  <div>
                    <div className="font-medium text-[var(--text)]">{user.user}</div>
                    <div className="text-xs text-[var(--muted)]">{user.most_common_change}</div>
                  </div>
                  <div className="text-right">
                    <div className="font-bold text-[var(--text)]">{user.total_overrides}</div>
                    <div className="text-xs text-[var(--muted)]">overrides</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        {/* Recent Activity */}
        <section className="rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[0_16px_40px_rgba(18,18,18,0.06)]">
          <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Recent Overrides</h3>

          {recentOverrides.length === 0 ? (
            <p className="text-sm text-[var(--muted)]">No recent overrides.</p>
          ) : (
            <div className="space-y-3">
              {recentOverrides.map((override) => (
                <div key={override.id} className="rounded-xl border border-[var(--line)] bg-[var(--surface-2)] p-3">
                  <div className="flex items-center justify-between">
                    <span className="font-medium text-[var(--text)]">{override.domain}</span>
                    <span className="text-xs text-[var(--muted)]">
                      {dayjs(override.created_at).fromNow()}
                    </span>
                  </div>
                  <div className="mt-1 flex items-center gap-2 text-sm">
                    <RecommendationBadge recommendation={override.original_recommendation} size="sm" />
                    <span className="text-[var(--muted)]">→</span>
                    <RecommendationBadge recommendation={override.override_recommendation} size="sm" />
                    <span className="text-xs text-[var(--muted)]">by {override.overridden_by}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      {/* Overrides Over Time */}
      {stats.overrides_over_time.length > 0 && (
        <section className="rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[0_16px_40px_rgba(18,18,18,0.06)]">
          <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Override Trend (Last 30 Days)</h3>
          <div className="h-48">
            <SimpleChart data={stats.overrides_over_time} />
          </div>
        </section>
      )}
    </div>
  );
}

function MetricCard({
  title,
  value,
  subtitle,
  trend
}: {
  title: string;
  value: string;
  subtitle: string;
  trend?: 'good' | 'neutral' | 'bad';
}) {
  return (
    <div className="rounded-3xl border border-[var(--line)] bg-[var(--surface)] p-6 shadow-[0_16px_40px_rgba(18,18,18,0.06)]">
      <div className="text-sm text-[var(--muted)]">{title}</div>
      <div className="mt-2 flex items-baseline gap-2">
        <span className="text-3xl font-bold text-[var(--text)]">{value}</span>
        {trend && (
          <span
            className={`text-sm ${
              trend === 'good' ? 'text-green-600' : trend === 'bad' ? 'text-red-600' : 'text-amber-600'
            }`}
          >
            {trend === 'good' ? '✓' : trend === 'bad' ? '!' : '~'}
          </span>
        )}
      </div>
      <div className="mt-1 text-xs text-[var(--muted)]">{subtitle}</div>
    </div>
  );
}

function RecommendationBadge({
  recommendation,
  size = 'md'
}: {
  recommendation: string;
  size?: 'sm' | 'md';
}) {
  const colors: Record<string, string> = {
    BLOCK: 'bg-red-100 text-red-700',
    REVIEW: 'bg-amber-100 text-amber-700',
    ALLOW_WITH_CAUTION: 'bg-yellow-100 text-yellow-700',
    ALLOW: 'bg-green-100 text-green-700'
  };

  const labels: Record<string, string> = {
    BLOCK: 'Block',
    REVIEW: 'Review',
    ALLOW_WITH_CAUTION: 'Caution',
    ALLOW: 'Allow'
  };

  const padding = size === 'sm' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-1 text-xs';

  return (
    <span className={`inline-flex items-center rounded-full font-medium ${colors[recommendation] ?? 'bg-gray-100 text-gray-700'} ${padding}`}>
      {labels[recommendation] ?? recommendation}
    </span>
  );
}

function SimpleChart({ data }: { data: { date: string; value?: number }[] }) {
  if (data.length === 0) return null;

  const values = data.map((d) => d.value ?? 0);
  const maxValue = Math.max(...values, 1);

  return (
    <div className="flex h-full items-end gap-1">
      {data.map((point, idx) => {
        const height = ((point.value ?? 0) / maxValue) * 100;
        return (
          <div key={idx} className="group relative flex-1">
            <div
              className="w-full rounded-t bg-purple-500 transition-all hover:bg-purple-600"
              style={{ height: `${Math.max(height, 2)}%` }}
            />
            <div className="absolute -top-8 left-1/2 hidden -translate-x-1/2 whitespace-nowrap rounded bg-[var(--text)] px-2 py-1 text-xs text-white group-hover:block">
              {dayjs(point.date).format('MMM D')}: {point.value?.toFixed(0)}
            </div>
          </div>
        );
      })}
    </div>
  );
}
