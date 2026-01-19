import { useEffect, useState } from 'react';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import type { FeedbackStatsResponse, OverrideDTO, TrainingTermDTO, TrainingTermCategory } from '../types';
import { fetchFeedbackStats, fetchOverrides, fetchTrainingTerms, createTrainingTermsBulk, deleteTrainingTerm } from '../lib/api';

dayjs.extend(relativeTime);

interface AILearningDashboardProps {
  batchId?: number;
}

export default function AILearningDashboard({ batchId }: AILearningDashboardProps) {
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState<FeedbackStatsResponse | null>(null);
  const [recentOverrides, setRecentOverrides] = useState<OverrideDTO[]>([]);
  const [trademarkTerms, setTrademarkTerms] = useState<TrainingTermCategory>({ yes_risk: [], no_risk: [] });
  const [viceTerms, setViceTerms] = useState<TrainingTermCategory>({ yes_risk: [], no_risk: [] });
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'trademark' | 'vice'>('trademark');

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
        setTrademarkTerms(termsData.trademark || { yes_risk: [], no_risk: [] });
        setViceTerms(termsData.vice || { yes_risk: [], no_risk: [] });
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load AI learning data');
      } finally {
        setLoading(false);
      }
    }
    loadData();
  }, [batchId]);

  const handleAddTermsBulk = async (
    terms: string[],
    classification: 'YES_RISK' | 'NO_RISK',
    category: 'trademark' | 'vice'
  ) => {
    try {
      const result = await createTrainingTermsBulk(terms, classification, category);
      // Refresh terms after bulk add
      const termsData = await fetchTrainingTerms();
      setTrademarkTerms(termsData.trademark || { yes_risk: [], no_risk: [] });
      setViceTerms(termsData.vice || { yes_risk: [], no_risk: [] });
      return result;
    } catch (err) {
      throw err;
    }
  };

  const handleDeleteTerm = async (id: number, classification: 'YES_RISK' | 'NO_RISK', category: 'trademark' | 'vice') => {
    try {
      await deleteTrainingTerm(id);
      if (category === 'trademark') {
        setTrademarkTerms(prev => ({
          yes_risk: classification === 'YES_RISK' ? prev.yes_risk.filter(t => t.id !== id) : prev.yes_risk,
          no_risk: classification === 'NO_RISK' ? prev.no_risk.filter(t => t.id !== id) : prev.no_risk
        }));
      } else {
        setViceTerms(prev => ({
          yes_risk: classification === 'YES_RISK' ? prev.yes_risk.filter(t => t.id !== id) : prev.yes_risk,
          no_risk: classification === 'NO_RISK' ? prev.no_risk.filter(t => t.id !== id) : prev.no_risk
        }));
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

  const currentTerms = activeTab === 'trademark' ? trademarkTerms : viceTerms;

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

      {/* Training Terms - Tabbed with Bulk Input */}
      <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-6">
        <h3 className="mb-2 text-lg font-semibold text-[var(--text)]">Teach AI</h3>
        <p className="mb-4 text-sm text-[var(--muted)]">Add terms to guide AI in future evaluations. Use comma or newline to add multiple terms at once.</p>

        {/* Category Tabs */}
        <div className="flex gap-2 mb-4">
          <button
            onClick={() => setActiveTab('trademark')}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
              activeTab === 'trademark'
                ? 'bg-blue-500 text-white'
                : 'bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--surface-3)]'
            }`}
          >
            Trademark Risk
            <span className="ml-2 text-xs opacity-75">
              ({trademarkTerms.yes_risk.length + trademarkTerms.no_risk.length})
            </span>
          </button>
          <button
            onClick={() => setActiveTab('vice')}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
              activeTab === 'vice'
                ? 'bg-purple-500 text-white'
                : 'bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--surface-3)]'
            }`}
          >
            Vice Risk
            <span className="ml-2 text-xs opacity-75">
              ({viceTerms.yes_risk.length + viceTerms.no_risk.length})
            </span>
          </button>
        </div>

        {/* Two Columns for YES_RISK and NO_RISK */}
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <TermColumnBulk
            title="YES_RISK"
            subtitle={activeTab === 'trademark' ? 'Terms that indicate trademark infringement' : 'Terms that indicate vice/adult content'}
            terms={currentTerms.yes_risk}
            onAddBulk={(terms) => handleAddTermsBulk(terms, 'YES_RISK', activeTab)}
            onDelete={(id) => handleDeleteTerm(id, 'YES_RISK', activeTab)}
            color="red"
          />
          <TermColumnBulk
            title="NO_RISK"
            subtitle={activeTab === 'trademark' ? 'Terms that are safe/generic' : 'Terms that are not vice-related'}
            terms={currentTerms.no_risk}
            onAddBulk={(terms) => handleAddTermsBulk(terms, 'NO_RISK', activeTab)}
            onDelete={(id) => handleDeleteTerm(id, 'NO_RISK', activeTab)}
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

function TermColumnBulk({
  title,
  subtitle,
  terms,
  onAddBulk,
  onDelete,
  color
}: {
  title: string;
  subtitle: string;
  terms: TrainingTermDTO[];
  onAddBulk: (terms: string[]) => Promise<{ created: number; skipped: number }>;
  onDelete: (id: number) => void;
  color: 'red' | 'green';
}) {
  const [inputValue, setInputValue] = useState('');
  const [isAdding, setIsAdding] = useState(false);
  const [feedback, setFeedback] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputValue.trim()) return;

    // Parse input - split by comma, newline, or semicolon
    const termsToAdd = inputValue
      .split(/[,\n;]+/)
      .map(t => t.trim().toLowerCase())
      .filter(t => t.length > 0);

    if (termsToAdd.length === 0) return;

    setIsAdding(true);
    setFeedback(null);

    try {
      const result = await onAddBulk(termsToAdd);
      setInputValue('');
      if (result.created > 0 || result.skipped > 0) {
        setFeedback(`Added ${result.created} term${result.created !== 1 ? 's' : ''}${result.skipped > 0 ? `, ${result.skipped} skipped (duplicates)` : ''}`);
        setTimeout(() => setFeedback(null), 3000);
      }
    } catch (err) {
      setFeedback(err instanceof Error ? err.message : 'Failed to add terms');
    } finally {
      setIsAdding(false);
    }
  };

  const bgColor = color === 'red' ? 'bg-red-50' : 'bg-green-50';
  const borderColor = color === 'red' ? 'border-red-200' : 'border-green-200';
  const textColor = color === 'red' ? 'text-red-700' : 'text-green-700';
  const buttonBg = color === 'red' ? 'bg-red-500 hover:bg-red-600' : 'bg-green-500 hover:bg-green-600';

  return (
    <div className={`rounded-lg border ${borderColor} ${bgColor} p-4`}>
      <h4 className={`font-semibold ${textColor} mb-1`}>{title}</h4>
      <p className="text-xs text-[var(--muted)] mb-3">{subtitle}</p>

      <form onSubmit={handleSubmit} className="mb-3">
        <textarea
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          placeholder="Enter terms (comma, newline, or semicolon separated)&#10;e.g.: term1, term2, term3"
          rows={3}
          className="w-full rounded-lg border border-[var(--line)] bg-white px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-[var(--accent)] resize-none"
        />
        <div className="flex items-center justify-between mt-2">
          <span className="text-xs text-[var(--muted)]">
            {inputValue.trim() ? `${inputValue.split(/[,\n;]+/).filter(t => t.trim()).length} terms` : ''}
          </span>
          <button
            type="submit"
            disabled={!inputValue.trim() || isAdding}
            className={`rounded-lg px-4 py-2 text-sm font-medium text-white ${buttonBg} disabled:opacity-50`}
          >
            {isAdding ? 'Adding...' : 'Add Terms'}
          </button>
        </div>
        {feedback && (
          <p className={`text-xs mt-2 ${feedback.includes('Failed') ? 'text-red-600' : 'text-green-600'}`}>
            {feedback}
          </p>
        )}
      </form>

      <div className="space-y-2 max-h-48 overflow-y-auto">
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
      {terms.length > 0 && (
        <p className="text-xs text-[var(--muted)] mt-2">{terms.length} term{terms.length !== 1 ? 's' : ''}</p>
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
