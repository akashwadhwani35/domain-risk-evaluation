import { useEffect, useState } from 'react';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import type { FeedbackStatsResponse, OverrideDTO, TrainingTermDTO } from '../types';
import { fetchFeedbackStats, fetchOverrides, fetchTrainingTerms, createTrainingTerm, deleteTrainingTerm } from '../lib/api';

dayjs.extend(relativeTime);

interface AILearningDashboardProps {
  batchId?: number;
}

export default function AILearningDashboard({ batchId }: AILearningDashboardProps) {
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState<FeedbackStatsResponse | null>(null);
  const [recentOverrides, setRecentOverrides] = useState<OverrideDTO[]>([]);
  const [yesRiskTerms, setYesRiskTerms] = useState<TrainingTermDTO[]>([]);
  const [noRiskTerms, setNoRiskTerms] = useState<TrainingTermDTO[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function loadData() {
      setLoading(true);
      setError(null);
      try {
        const [statsData, overridesData, termsData] = await Promise.all([
          fetchFeedbackStats(batchId),
          fetchOverrides(0, 10),
          fetchTrainingTerms()
        ]);
        setStats(statsData);
        setRecentOverrides(overridesData.items);
        setYesRiskTerms(termsData.yes_risk);
        setNoRiskTerms(termsData.no_risk);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load AI learning data');
      } finally {
        setLoading(false);
      }
    }
    loadData();
  }, [batchId]);

  const handleAddTerm = async (term: string, classification: 'YES_RISK' | 'NO_RISK') => {
    try {
      const newTerm = await createTrainingTerm(term, classification);
      if (classification === 'YES_RISK') {
        setYesRiskTerms(prev => [...prev, newTerm]);
      } else {
        setNoRiskTerms(prev => [...prev, newTerm]);
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to add term');
    }
  };

  const handleDeleteTerm = async (id: number, classification: 'YES_RISK' | 'NO_RISK') => {
    try {
      await deleteTrainingTerm(id);
      if (classification === 'YES_RISK') {
        setYesRiskTerms(prev => prev.filter(t => t.id !== id));
      } else {
        setNoRiskTerms(prev => prev.filter(t => t.id !== id));
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to delete term');
    }
  };

  if (loading) {
    return (
      <div className="flex h-96 items-center justify-center">
        <div className="text-[var(--muted)]">Loading AI learning data...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-xl border border-red-200 bg-red-50 p-8">
        <p className="text-red-600">{error}</p>
      </div>
    );
  }

  if (!stats) {
    return null;
  }

  return (
    <div className="space-y-6">
      {/* Overview Metrics */}
      <section className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <MetricCard
          title="Total Overrides"
          value={stats.total_overrides.toLocaleString()}
          subtitle={`${stats.total_feedback_embeddings} embeddings`}
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
          subtitle="avg retrievals"
        />
      </section>

      {/* Training Terms - Two Columns */}
      <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-6">
        <h3 className="mb-2 text-lg font-semibold text-[var(--text)]">Teach AI</h3>
        <p className="mb-4 text-sm text-[var(--muted)]">Add terms to guide AI in future evaluations</p>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {/* YES_RISK Column */}
          <TermColumn
            title="YES_RISK"
            terms={yesRiskTerms}
            onAdd={(term) => handleAddTerm(term, 'YES_RISK')}
            onDelete={(id) => handleDeleteTerm(id, 'YES_RISK')}
            color="red"
          />

          {/* NO_RISK Column */}
          <TermColumn
            title="NO_RISK"
            terms={noRiskTerms}
            onAdd={(term) => handleAddTerm(term, 'NO_RISK')}
            onDelete={(id) => handleDeleteTerm(id, 'NO_RISK')}
            color="green"
          />
        </div>
      </section>

      {/* Correction Patterns */}
      <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-6">
        <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Correction Patterns</h3>

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
      </section>

      {/* Recent Overrides */}
      <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-6">
        <h3 className="mb-4 text-lg font-semibold text-[var(--text)]">Recent Overrides</h3>

        {recentOverrides.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No recent overrides.</p>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {recentOverrides.map((override) => (
              <div key={override.id} className="rounded-lg border border-[var(--line)] bg-[var(--surface-2)] p-3">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-[var(--text)] text-sm truncate">{override.domain}</span>
                  <span className="text-xs text-[var(--muted)]">
                    {dayjs(override.created_at).fromNow()}
                  </span>
                </div>
                <div className="mt-1 flex items-center gap-2 text-sm">
                  <span className="text-xs text-[var(--muted)]">TM:</span>
                  <RecommendationBadge recommendation={override.original_trademark_recommendation} size="sm" />
                  <span className="text-[var(--muted)]">→</span>
                  <RecommendationBadge recommendation={override.override_trademark_recommendation} size="sm" />
                </div>
                <div className="mt-1 flex items-center gap-2 text-sm">
                  <span className="text-xs text-[var(--muted)]">Vice:</span>
                  <RecommendationBadge recommendation={override.original_vice_recommendation} size="sm" />
                  <span className="text-[var(--muted)]">→</span>
                  <RecommendationBadge recommendation={override.override_vice_recommendation} size="sm" />
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function TermColumn({
  title,
  terms,
  onAdd,
  onDelete,
  color
}: {
  title: string;
  terms: TrainingTermDTO[];
  onAdd: (term: string) => void;
  onDelete: (id: number) => void;
  color: 'red' | 'green';
}) {
  const [newTerm, setNewTerm] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (newTerm.trim()) {
      onAdd(newTerm.trim());
      setNewTerm('');
    }
  };

  const bgColor = color === 'red' ? 'bg-red-50' : 'bg-green-50';
  const borderColor = color === 'red' ? 'border-red-200' : 'border-green-200';
  const textColor = color === 'red' ? 'text-red-700' : 'text-green-700';
  const buttonBg = color === 'red' ? 'bg-red-500 hover:bg-red-600' : 'bg-green-500 hover:bg-green-600';

  return (
    <div className={`rounded-lg border ${borderColor} ${bgColor} p-4`}>
      <h4 className={`font-semibold ${textColor} mb-3`}>{title}</h4>

      <form onSubmit={handleSubmit} className="flex gap-2 mb-3">
        <input
          type="text"
          value={newTerm}
          onChange={(e) => setNewTerm(e.target.value)}
          placeholder="Add term..."
          className="flex-1 rounded-lg border border-[var(--line)] bg-white px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
        />
        <button
          type="submit"
          disabled={!newTerm.trim()}
          className={`rounded-lg px-3 py-2 text-sm font-medium text-white ${buttonBg} disabled:opacity-50`}
        >
          +
        </button>
      </form>

      <div className="space-y-2 max-h-64 overflow-y-auto">
        {terms.length === 0 ? (
          <p className="text-sm text-[var(--muted)] italic">No terms yet</p>
        ) : (
          terms.map((term) => (
            <div
              key={term.id}
              className="flex items-center justify-between rounded-lg bg-white px-3 py-2 border border-[var(--line)]"
            >
              <span className="text-sm text-[var(--text)]">{term.term}</span>
              <button
                type="button"
                onClick={() => onDelete(term.id)}
                className="text-[var(--muted)] hover:text-red-500 text-lg leading-none"
              >
                ×
              </button>
            </div>
          ))
        )}
      </div>
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
    <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4">
      <div className="text-xs text-[var(--muted)]">{title}</div>
      <div className="mt-1 flex items-baseline gap-2">
        <span className="text-2xl font-bold text-[var(--text)]">{value}</span>
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
      <div className="text-xs text-[var(--muted)]">{subtitle}</div>
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
    YES_RISK: 'bg-red-100 text-red-700',
    POTENTIAL_RISK: 'bg-amber-100 text-amber-700',
    NO_RISK: 'bg-green-100 text-green-700',
    BLOCK: 'bg-red-100 text-red-700',
    REVIEW: 'bg-amber-100 text-amber-700',
    ALLOW: 'bg-green-100 text-green-700'
  };

  const labels: Record<string, string> = {
    YES_RISK: 'Yes',
    POTENTIAL_RISK: 'Potential',
    NO_RISK: 'No',
    BLOCK: 'Yes',
    REVIEW: 'Potential',
    ALLOW: 'No'
  };

  const padding = size === 'sm' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-1 text-xs';

  return (
    <span className={`inline-flex items-center rounded-full font-medium ${colors[recommendation] ?? 'bg-gray-100 text-gray-700'} ${padding}`}>
      {labels[recommendation] ?? recommendation}
    </span>
  );
}
