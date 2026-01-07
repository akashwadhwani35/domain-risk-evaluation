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
  { value: 'YES_RISK', label: 'Yes Risk', color: 'bg-red-500' },
  { value: 'POTENTIAL_RISK', label: 'Potential', color: 'bg-amber-500' },
  { value: 'NO_RISK', label: 'No Risk', color: 'bg-green-500' }
];

export default function OverrideModal({ evaluation, onClose, onOverrideCreated }: OverrideModalProps) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [history, setHistory] = useState<OverrideDTO[]>([]);

  // Form state - just the two recommendations
  const [trademarkRecommendation, setTrademarkRecommendation] = useState(
    normalizeRecommendation(evaluation.trademark_recommendation)
  );
  const [viceRecommendation, setViceRecommendation] = useState(
    normalizeRecommendation(evaluation.vice_recommendation)
  );

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
    setSubmitting(true);
    setError(null);

    const request: OverrideRequest = {
      overridden_by: 'User',
      reason: 'Manual correction'
    };

    // Only include trademark recommendation if changed
    const normalizedTmRec = normalizeRecommendation(evaluation.trademark_recommendation);
    if (trademarkRecommendation !== normalizedTmRec) {
      request.override_trademark_recommendation = trademarkRecommendation;
    }

    // Only include vice recommendation if changed
    const normalizedViceRec = normalizeRecommendation(evaluation.vice_recommendation);
    if (viceRecommendation !== normalizedViceRec) {
      request.override_vice_recommendation = viceRecommendation;
    }

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

  const normalizedTmRec = normalizeRecommendation(evaluation.trademark_recommendation);
  const normalizedViceRec = normalizeRecommendation(evaluation.vice_recommendation);
  const hasChanges =
    trademarkRecommendation !== normalizedTmRec ||
    viceRecommendation !== normalizedViceRec;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
      <div className="relative w-full max-w-md rounded-2xl border border-[var(--line)] bg-[var(--surface)] shadow-2xl">
        <button
          type="button"
          onClick={onClose}
          className="absolute right-3 top-3 rounded-full p-1.5 text-[var(--muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)]"
        >
          <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>

        <div className="p-6">
          {/* Domain Header */}
          <div className="mb-5">
            <p className="text-xs text-[var(--muted)] uppercase tracking-wider">Override</p>
            <h2 className="text-lg font-semibold text-[var(--text)] break-all">{evaluation.domain}</h2>
          </div>

          <form onSubmit={handleSubmit} className="space-y-5">
            {/* Trademark */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="text-sm font-medium text-[var(--text)]">Trademark</label>
                <ScoreBadge label={evaluation.trademark_recommendation} />
              </div>
              <div className="grid grid-cols-3 gap-2">
                {RECOMMENDATIONS.map((rec) => (
                  <button
                    key={rec.value}
                    type="button"
                    onClick={() => setTrademarkRecommendation(rec.value)}
                    className={clsx(
                      'rounded-lg px-3 py-2 text-sm font-medium transition-all border',
                      trademarkRecommendation === rec.value
                        ? `${rec.color} text-white border-transparent`
                        : 'border-[var(--line)] bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--surface)]'
                    )}
                  >
                    {rec.label}
                  </button>
                ))}
              </div>
            </div>

            {/* Vice */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="text-sm font-medium text-[var(--text)]">Vice</label>
                <ScoreBadge label={evaluation.vice_recommendation} />
              </div>
              <div className="grid grid-cols-3 gap-2">
                {RECOMMENDATIONS.map((rec) => (
                  <button
                    key={rec.value}
                    type="button"
                    onClick={() => setViceRecommendation(rec.value)}
                    className={clsx(
                      'rounded-lg px-3 py-2 text-sm font-medium transition-all border',
                      viceRecommendation === rec.value
                        ? `${rec.color} text-white border-transparent`
                        : 'border-[var(--line)] bg-[var(--surface-2)] text-[var(--text)] hover:bg-[var(--surface)]'
                    )}
                  >
                    {rec.label}
                  </button>
                ))}
              </div>
            </div>

            {error && (
              <div className="rounded-lg bg-red-50 p-3 text-sm text-red-600">
                {error}
              </div>
            )}

            {/* Actions */}
            <div className="flex gap-3 pt-2">
              <button
                type="button"
                onClick={onClose}
                className="flex-1 rounded-lg border border-[var(--line)] px-4 py-2.5 text-sm font-medium text-[var(--text)] hover:bg-[var(--surface-2)]"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={submitting || !hasChanges}
                className={clsx(
                  'flex-1 rounded-lg px-4 py-2.5 text-sm font-medium text-white',
                  submitting || !hasChanges
                    ? 'cursor-not-allowed bg-gray-400'
                    : 'bg-[var(--text)] hover:opacity-90'
                )}
              >
                {submitting ? 'Saving...' : 'Save'}
              </button>
            </div>
          </form>

          {/* History (collapsed) */}
          {history.length > 0 && (
            <div className="mt-5 pt-4 border-t border-[var(--line)]">
              <p className="text-xs text-[var(--muted)] mb-2">{history.length} previous override{history.length > 1 ? 's' : ''}</p>
              <div className="text-xs text-[var(--muted)]">
                Last: {dayjs(history[0].created_at).format('MMM D, HH:mm')} — TM: {formatDecision(history[0].override_trademark_recommendation)}, Vice: {formatDecision(history[0].override_vice_recommendation)}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

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
      return 'Potential';
    default:
      return rec;
  }
}
