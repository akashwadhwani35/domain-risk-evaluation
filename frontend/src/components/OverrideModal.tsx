import { useState, useEffect } from 'react';
import clsx from 'clsx';
import dayjs from 'dayjs';
import ScoreBadge from './ScoreBadge';
import type { EvaluationDTO, OverrideDTO, OverrideRequest } from '../types';
import { createOverride, fetchEvaluationHistory } from '../lib/api';

interface OverrideModalProps {
  evaluation: EvaluationDTO;
  onClose: () => void;
  onOverrideCreated: (override: OverrideDTO) => void;
}

const RECOMMENDATIONS = [
  { value: 'YES_RISK', label: 'Yes Risk', color: 'bg-red-500', description: 'Famous brand - block registration' },
  { value: 'POTENTIAL_RISK', label: 'Potential Risk', color: 'bg-amber-500', description: 'Ambiguous - needs human review' },
  { value: 'NO_RISK', label: 'No Risk', color: 'bg-green-500', description: 'Common word - safe to register' }
];

const OVERRIDE_REASONS = [
  'This is a famous brand name - should be blocked',
  'This is a common English word - not a trademark risk',
  'Ambiguous case - needs human review',
  'Vice content misclassified',
  'Commercial precedent exists',
  'Policy exception',
  'Other'
];

export default function OverrideModal({ evaluation, onClose, onOverrideCreated }: OverrideModalProps) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [history, setHistory] = useState<OverrideDTO[]>([]);

  // Form state
  const [userName, setUserName] = useState('');
  const [selectedRecommendation, setSelectedRecommendation] = useState(
    normalizeRecommendation(evaluation.overall_recommendation)
  );
  const [reason, setReason] = useState('');
  const [customReason, setCustomReason] = useState('');
  const [explanation, setExplanation] = useState(evaluation.explanation);

  useEffect(() => {
    async function loadHistory() {
      try {
        const response = await fetchEvaluationHistory(evaluation.id);
        setHistory(response.overrides);
      } catch {
        // History not critical - continue without it
      }
    }
    loadHistory();
  }, [evaluation.id]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!userName.trim()) {
      setError('Please enter your name');
      return;
    }
    if (!reason) {
      setError('Please select a reason');
      return;
    }

    setSubmitting(true);
    setError(null);

    const finalReason = reason === 'Other' ? customReason : reason;
    const request: OverrideRequest = {
      overridden_by: userName.trim(),
      reason: finalReason,
      override_recommendation: selectedRecommendation,
      override_explanation: explanation !== evaluation.explanation ? explanation : undefined
    };

    try {
      const override = await createOverride(evaluation.id, request);
      onOverrideCreated(override);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create override');
    } finally {
      setSubmitting(false);
    }
  };

  const normalizedCurrentRec = normalizeRecommendation(evaluation.overall_recommendation);
  const hasChanges =
    selectedRecommendation !== normalizedCurrentRec ||
    explanation !== evaluation.explanation;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="relative max-h-[90vh] w-full max-w-3xl overflow-y-auto rounded-3xl border border-[var(--line)] bg-[var(--surface)] shadow-2xl">
        <button
          type="button"
          onClick={onClose}
          className="absolute right-4 top-4 rounded-full p-2 text-[var(--muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)]"
        >
          <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>

        <div className="p-8">
          <h2 className="mb-6 text-xl font-semibold text-[var(--text)]">Override Decision</h2>

          {/* Current Evaluation */}
          <section className="mb-8 rounded-2xl border border-[var(--line)] bg-[var(--surface-2)] p-6">
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-[var(--muted)]">Current Evaluation</h3>
            <div className="flex flex-wrap items-start gap-6">
              <div>
                <span className="text-xs text-[var(--muted)]">Domain</span>
                <p className="font-medium text-[var(--text)]">{evaluation.domain}</p>
              </div>
              <div>
                <span className="text-xs text-[var(--muted)]">Current Decision</span>
                <div className="mt-1">
                  <ScoreBadge label={evaluation.overall_recommendation} />
                </div>
              </div>
              <div>
                <span className="text-xs text-[var(--muted)]">Confidence</span>
                <p className="font-medium text-[var(--text)]">{(evaluation.confidence * 100).toFixed(0)}%</p>
              </div>
            </div>
            <div className="mt-4">
              <span className="text-xs text-[var(--muted)]">AI Explanation</span>
              <p className="mt-1 whitespace-pre-line text-sm text-[var(--text)]">{evaluation.explanation}</p>
            </div>
          </section>

          {/* Override Form */}
          <form onSubmit={handleSubmit} className="space-y-6">
            <div>
              <label className="mb-2 block text-sm font-medium text-[var(--text)]">Your Name</label>
              <input
                type="text"
                value={userName}
                onChange={(e) => setUserName(e.target.value)}
                placeholder="Enter your name"
                className="w-full rounded-2xl border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--text)] focus:outline-none"
              />
            </div>

            <div>
              <label className="mb-3 block text-sm font-medium text-[var(--text)]">Override Decision</label>
              <div className="grid gap-3">
                {RECOMMENDATIONS.map((rec) => (
                  <button
                    key={rec.value}
                    type="button"
                    onClick={() => setSelectedRecommendation(rec.value)}
                    className={clsx(
                      'flex items-center gap-4 rounded-xl px-4 py-3 text-left transition-all border',
                      selectedRecommendation === rec.value
                        ? `${rec.color} text-white border-transparent shadow-md`
                        : 'border-[var(--line)] bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--surface)]'
                    )}
                  >
                    <div className={clsx(
                      'w-4 h-4 rounded-full border-2 flex items-center justify-center',
                      selectedRecommendation === rec.value
                        ? 'border-white bg-white/30'
                        : 'border-[var(--muted)]'
                    )}>
                      {selectedRecommendation === rec.value && (
                        <div className="w-2 h-2 rounded-full bg-white" />
                      )}
                    </div>
                    <div>
                      <div className="font-medium">{rec.label}</div>
                      <div className={clsx(
                        'text-xs',
                        selectedRecommendation === rec.value ? 'text-white/80' : 'text-[var(--muted)]'
                      )}>
                        {rec.description}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-[var(--text)]">Override Reason</label>
              <select
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                className="w-full rounded-2xl border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3 text-sm text-[var(--text)] focus:border-[var(--text)] focus:outline-none"
              >
                <option value="">Select a reason...</option>
                {OVERRIDE_REASONS.map((r) => (
                  <option key={r} value={r}>{r}</option>
                ))}
              </select>
              {reason === 'Other' && (
                <input
                  type="text"
                  value={customReason}
                  onChange={(e) => setCustomReason(e.target.value)}
                  placeholder="Enter custom reason"
                  className="mt-2 w-full rounded-2xl border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--text)] focus:outline-none"
                />
              )}
            </div>

            <div>
              <label className="mb-2 block text-sm font-medium text-[var(--text)]">Corrected Explanation</label>
              <textarea
                value={explanation}
                onChange={(e) => setExplanation(e.target.value)}
                rows={4}
                placeholder="Provide a corrected explanation for AI learning..."
                className="w-full resize-none rounded-2xl border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--text)] focus:outline-none"
              />
              <p className="mt-1 text-xs text-[var(--muted)]">
                This explanation will be used to train the AI for similar domains.
              </p>
            </div>

            {error && (
              <div className="rounded-xl bg-red-50 p-4 text-sm text-red-600">
                {error}
              </div>
            )}

            <div className="flex items-center justify-between border-t border-[var(--line)] pt-6">
              <button
                type="button"
                onClick={onClose}
                className="rounded-full border border-[var(--line)] px-6 py-2.5 text-sm font-medium text-[var(--text)] hover:bg-[var(--surface-2)]"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={submitting || !hasChanges}
                className={clsx(
                  'rounded-full px-6 py-2.5 text-sm font-medium text-white',
                  submitting || !hasChanges
                    ? 'cursor-not-allowed bg-gray-400'
                    : 'bg-[var(--text)] hover:-translate-y-0.5 hover:shadow-md'
                )}
              >
                {submitting ? 'Submitting...' : 'Submit Override'}
              </button>
            </div>
          </form>

          {/* Override History */}
          {history.length > 0 && (
            <section className="mt-8 border-t border-[var(--line)] pt-6">
              <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-[var(--muted)]">Override History</h3>
              <div className="space-y-4">
                {history.map((override) => (
                  <div key={override.id} className="rounded-xl border border-[var(--line)] bg-[var(--surface-2)] p-4">
                    <div className="flex flex-wrap items-center gap-4 text-sm">
                      <span className="font-medium text-[var(--text)]">{override.overridden_by}</span>
                      <span className="text-[var(--muted)]">{dayjs(override.created_at).format('YYYY-MM-DD HH:mm')}</span>
                      <span className="text-[var(--muted)]">
                        {formatDecision(override.original_recommendation)} → {formatDecision(override.override_recommendation)}
                      </span>
                      {override.feedback_applied && (
                        <span className="rounded-full bg-green-100 px-2 py-0.5 text-xs text-green-700">
                          AI Learning Applied
                        </span>
                      )}
                    </div>
                    <p className="mt-2 text-sm text-[var(--muted)]">{override.reason}</p>
                  </div>
                ))}
              </div>
            </section>
          )}
        </div>
      </div>
    </div>
  );
}

// Helper to normalize legacy recommendations to new 3-tier format
function normalizeRecommendation(rec: string): string {
  switch (rec.toUpperCase()) {
    case 'YES_RISK':
    case 'BLOCK':
      return 'YES_RISK';
    case 'NO_RISK':
    case 'ALLOW':
      return 'NO_RISK';
    case 'POTENTIAL_RISK':
    case 'REVIEW':
    case 'ALLOW_WITH_CAUTION':
      return 'POTENTIAL_RISK';
    default:
      return 'POTENTIAL_RISK';
  }
}

function formatDecision(rec: string): string {
  switch (rec.toUpperCase()) {
    case 'YES_RISK':
    case 'BLOCK':
      return 'Yes Risk';
    case 'NO_RISK':
    case 'ALLOW':
      return 'No Risk';
    case 'POTENTIAL_RISK':
    case 'REVIEW':
    case 'ALLOW_WITH_CAUTION':
      return 'Potential Risk';
    default:
      return rec;
  }
}
