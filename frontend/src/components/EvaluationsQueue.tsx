import { useCallback, useEffect, useState } from 'react';
import clsx from 'clsx';
import { fetchResults, createOverride, mapEvaluation } from '../lib/api';
import { HelpTip } from './Tooltip';
import type { EvaluationDTO, OverrideRequest } from '../types';

interface EvaluationsQueueProps {
  batchId?: number;
  onOverrideCreated?: () => void;
}

const PAGE_SIZE = 50;

const DECISIONS = [
  { value: 'YES_RISK', label: 'Yes', color: 'bg-red-500 hover:bg-red-600' },
  { value: 'NO_RISK', label: 'No', color: 'bg-green-500 hover:bg-green-600' }
];

function normalizeRecommendation(rec: string): string {
  switch (rec?.toUpperCase()) {
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

export default function EvaluationsQueue({ batchId, onOverrideCreated }: EvaluationsQueueProps) {
  const [evaluations, setEvaluations] = useState<EvaluationDTO[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [processingId, setProcessingId] = useState<number | null>(null);

  const loadEvaluations = useCallback(async () => {
    setLoading(true);
    try {
      // Fetch evaluations where EITHER trademark OR vice is POTENTIAL_RISK
      const response = await fetchResults({
        recommendation: 'POTENTIAL_RISK',
        sort: 'created_desc',
        page,
        pageSize: PAGE_SIZE,
        batchId
      });
      const mapped = response.items.map(mapEvaluation);
      setEvaluations(mapped);
      setTotal(response.total);
    } catch (err) {
      console.error('Failed to load evaluations', err);
    } finally {
      setLoading(false);
    }
  }, [page, batchId]);

  useEffect(() => {
    loadEvaluations();
  }, [loadEvaluations]);

  const handleOverride = async (
    evaluation: EvaluationDTO,
    tmRec: string,
    viceRec: string
  ) => {
    setProcessingId(evaluation.id);

    const request: OverrideRequest = {
      overridden_by: 'Quick Review',
      reason: 'Manual review decision',
      override_trademark_recommendation: tmRec,
      override_vice_recommendation: viceRec
    };

    try {
      await createOverride(evaluation.id, request);
      // Remove from list if neither is POTENTIAL_RISK anymore
      if (tmRec !== 'POTENTIAL_RISK' && viceRec !== 'POTENTIAL_RISK') {
        const newList = evaluations.filter(e => e.id !== evaluation.id);
        const newTotal = Math.max(0, total - 1);
        setEvaluations(newList);
        setTotal(newTotal);

        // If current page is now empty and we're not on first page, go back
        if (newList.length === 0 && page > 0) {
          setPage(page - 1);
        }
      } else {
        // Update the evaluation in place
        setEvaluations(prev => prev.map(e =>
          e.id === evaluation.id
            ? { ...e, trademark_recommendation: tmRec, vice_recommendation: viceRec }
            : e
        ));
      }
      onOverrideCreated?.();
    } catch (err) {
      console.error('Failed to create override', err);
    } finally {
      setProcessingId(null);
    }
  };

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  if (loading && evaluations.length === 0) {
    return (
      <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-12">
        <div className="flex items-center justify-center gap-2 text-[var(--muted)]">
          <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          Loading...
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h2 className="text-lg font-semibold text-[var(--text)]">Review Queue</h2>
            <HelpTip
              content="Domains marked as 'Maybe' by AI need your decision. Choose Yes (risky) or No (safe) for each category shown."
              position="right"
            />
          </div>
          <p className="text-sm text-[var(--muted)]">
            {total} domains need your decision
          </p>
        </div>
        <button
          type="button"
          onClick={() => loadEvaluations()}
          disabled={loading}
          className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm font-medium text-[var(--text)] hover:bg-[var(--surface-2)] disabled:opacity-50"
        >
          Refresh
        </button>
      </div>

      {/* Empty state */}
      {evaluations.length === 0 && !loading && (
        <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-12 text-center">
          <div className="text-4xl mb-4">🎉</div>
          <h3 className="text-lg font-semibold text-[var(--text)] mb-2">All caught up!</h3>
          <p className="text-[var(--muted)]">No domains need review right now.</p>
        </div>
      )}

      {/* Evaluation cards */}
      {evaluations.length > 0 && (
        <div className="space-y-3">
          {evaluations.map((evaluation) => {
            const isProcessing = processingId === evaluation.id;
            const currentTmRec = normalizeRecommendation(evaluation.trademark_recommendation);
            const currentViceRec = normalizeRecommendation(evaluation.vice_recommendation);

            return (
              <div
                key={evaluation.id}
                className={clsx(
                  'rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5 transition-opacity',
                  isProcessing && 'opacity-50 pointer-events-none'
                )}
              >
                {/* Domain info */}
                <div className="mb-4">
                  <h3 className="font-semibold text-[var(--text)] text-lg">{evaluation.domain}</h3>
                  <p className="mt-1 text-sm text-[var(--muted)] leading-relaxed line-clamp-2">
                    {evaluation.explanation || 'No explanation available'}
                  </p>
                  {evaluation.matched_trademark && (
                    <p className="mt-1 text-xs text-[var(--muted)]">
                      Matched: <span className="text-[var(--text)]">{evaluation.matched_trademark}</span>
                    </p>
                  )}
                </div>

                {/* Only show controls for what needs a decision */}
                <div className="flex flex-wrap items-center gap-3">
                  {/* Trademark - only if it's Maybe */}
                  {currentTmRec === 'POTENTIAL_RISK' && (
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-[var(--text)] flex items-center gap-1">
                        Trademark
                        <HelpTip content="Could this domain infringe on a brand name or trademark? Yes = block, No = safe" position="bottom" />
                        :
                      </span>
                      {DECISIONS.map((dec) => (
                        <button
                          key={dec.value}
                          type="button"
                          onClick={() => handleOverride(evaluation, dec.value, currentViceRec)}
                          disabled={isProcessing}
                          className={clsx(
                            'rounded-lg px-4 py-2 text-sm font-medium text-white transition-all',
                            dec.color,
                            'disabled:opacity-50 disabled:cursor-not-allowed'
                          )}
                        >
                          {dec.label}
                        </button>
                      ))}
                    </div>
                  )}

                  {/* Vice - only if it's Maybe */}
                  {currentViceRec === 'POTENTIAL_RISK' && (
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-[var(--text)] flex items-center gap-1">
                        Vice
                        <HelpTip content="Does this domain relate to adult content, gambling, drugs, or illegal activity? Yes = block, No = safe" position="bottom" />
                        :
                      </span>
                      {DECISIONS.map((dec) => (
                        <button
                          key={dec.value}
                          type="button"
                          onClick={() => handleOverride(evaluation, currentTmRec, dec.value)}
                          disabled={isProcessing}
                          className={clsx(
                            'rounded-lg px-4 py-2 text-sm font-medium text-white transition-all',
                            dec.color,
                            'disabled:opacity-50 disabled:cursor-not-allowed'
                          )}
                        >
                          {dec.label}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Pagination */}
      {total > PAGE_SIZE && (
        <div className="flex items-center justify-between border-t border-[var(--line)] pt-4">
          <span className="text-sm text-[var(--muted)]">
            Page {page + 1} of {totalPages}
          </span>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPage(Math.max(0, page - 1))}
              disabled={page === 0 || loading}
              className={clsx(
                'rounded-lg px-4 py-2 text-sm font-medium',
                page === 0 || loading
                  ? 'cursor-not-allowed bg-[var(--surface-2)] text-[var(--muted)]'
                  : 'bg-[var(--text)] text-white hover:opacity-90'
              )}
            >
              Previous
            </button>
            <button
              type="button"
              onClick={() => setPage(Math.min(totalPages - 1, page + 1))}
              disabled={page + 1 >= totalPages || loading}
              className={clsx(
                'rounded-lg px-4 py-2 text-sm font-medium',
                page + 1 >= totalPages || loading
                  ? 'cursor-not-allowed bg-[var(--surface-2)] text-[var(--muted)]'
                  : 'bg-[var(--text)] text-white hover:opacity-90'
              )}
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
