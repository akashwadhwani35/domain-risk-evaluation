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

type FilterType = 'all' | 'tm' | 'vice';

function normalizeRec(rec: string): string {
  switch (rec?.toUpperCase()) {
    case 'YES_RISK':
    case 'BLOCK':
      return 'YES_RISK';
    case 'NO_RISK':
    case 'ALLOW':
      return 'NO_RISK';
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
  const [filter, setFilter] = useState<FilterType>('all');

  const loadEvaluations = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetchResults({
        recommendation: 'POTENTIAL_RISK',
        recType: filter === 'all' ? undefined : filter,
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
  }, [page, batchId, filter]);

  useEffect(() => {
    loadEvaluations();
  }, [loadEvaluations]);

  // Reset page when filter changes
  useEffect(() => {
    setPage(0);
  }, [filter]);

  const handleOverride = async (
    evaluation: EvaluationDTO,
    category: 'tm' | 'vice',
    decision: 'YES_RISK' | 'NO_RISK'
  ) => {
    setProcessingId(evaluation.id);

    const currentTm = normalizeRec(evaluation.trademark_recommendation);
    const currentVice = normalizeRec(evaluation.vice_recommendation);

    const newTm = category === 'tm' ? decision : currentTm;
    const newVice = category === 'vice' ? decision : currentVice;

    const request: OverrideRequest = {
      overridden_by: 'Quick Review',
      reason: `${category === 'tm' ? 'Trademark' : 'Vice'}: ${decision === 'YES_RISK' ? 'Confirmed risk' : 'Confirmed safe'}`,
      override_trademark_recommendation: newTm,
      override_vice_recommendation: newVice
    };

    try {
      await createOverride(evaluation.id, request);

      // Check if this item should be removed from the list
      const stillNeedsReview =
        (filter === 'all' && (newTm === 'POTENTIAL_RISK' || newVice === 'POTENTIAL_RISK')) ||
        (filter === 'tm' && newTm === 'POTENTIAL_RISK') ||
        (filter === 'vice' && newVice === 'POTENTIAL_RISK');

      if (!stillNeedsReview) {
        const newList = evaluations.filter(e => e.id !== evaluation.id);
        const newTotal = Math.max(0, total - 1);
        setEvaluations(newList);
        setTotal(newTotal);

        if (newList.length === 0 && page > 0) {
          setPage(page - 1);
        }
      } else {
        // Update in place
        setEvaluations(prev => prev.map(e =>
          e.id === evaluation.id
            ? { ...e, trademark_recommendation: newTm, vice_recommendation: newVice }
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
              content="Domains marked as 'Maybe' by AI need your decision. Choose Yes (risky) or No (safe)."
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

      {/* Filter Tabs */}
      <div className="flex items-center gap-2">
        <div className="flex gap-1 p-1 rounded-lg bg-[var(--surface-2)]">
          {[
            { value: 'all', label: 'All' },
            { value: 'tm', label: 'Trademark' },
            { value: 'vice', label: 'Vice' }
          ].map((tab) => (
          <button
            key={tab.value}
            type="button"
            onClick={() => setFilter(tab.value as FilterType)}
            className={clsx(
              'px-4 py-2 text-sm font-medium rounded-md transition-all',
              filter === tab.value
                ? 'bg-[var(--surface)] text-[var(--text)] shadow-sm'
                : 'text-[var(--muted)] hover:text-[var(--text)]'
            )}
          >
            {tab.label}
          </button>
        ))}
        </div>
        <HelpTip
          content="Counts may overlap: a domain can need review for both Trademark AND Vice. The 'All' count shows unique domains, while TM + Vice may sum to more."
          position="right"
        />
      </div>

      {/* Empty state */}
      {evaluations.length === 0 && !loading && (
        <div className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-12 text-center">
          <div className="text-4xl mb-4">
            {filter === 'all' ? '' : filter === 'tm' ? '' : ''}
          </div>
          <h3 className="text-lg font-semibold text-[var(--text)] mb-2">All caught up!</h3>
          <p className="text-[var(--muted)]">
            No {filter === 'tm' ? 'trademark' : filter === 'vice' ? 'vice' : ''} domains need review right now.
          </p>
        </div>
      )}

      {/* Evaluation cards */}
      {evaluations.length > 0 && (
        <div className="space-y-3">
          {evaluations.map((evaluation) => {
            const isProcessing = processingId === evaluation.id;
            const tmNeedsReview = normalizeRec(evaluation.trademark_recommendation) === 'POTENTIAL_RISK';
            const viceNeedsReview = normalizeRec(evaluation.vice_recommendation) === 'POTENTIAL_RISK';

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

                {/* Category buttons with labels */}
                <div className="flex flex-wrap items-center gap-4">
                  {/* Trademark row - only show if TM needs review */}
                  {tmNeedsReview && (
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-[var(--text)] w-20">Trademark:</span>
                      <button
                        type="button"
                        onClick={() => handleOverride(evaluation, 'tm', 'YES_RISK')}
                        disabled={isProcessing}
                        className={clsx(
                          'rounded-lg px-3 py-1.5 text-sm font-medium transition-all',
                          'bg-red-100 text-red-700 hover:bg-red-200',
                          'disabled:opacity-50 disabled:cursor-not-allowed'
                        )}
                      >
                        Yes
                      </button>
                      <button
                        type="button"
                        onClick={() => handleOverride(evaluation, 'tm', 'NO_RISK')}
                        disabled={isProcessing}
                        className={clsx(
                          'rounded-lg px-3 py-1.5 text-sm font-medium transition-all',
                          'bg-green-100 text-green-700 hover:bg-green-200',
                          'disabled:opacity-50 disabled:cursor-not-allowed'
                        )}
                      >
                        No
                      </button>
                    </div>
                  )}

                  {/* Vice row - only show if Vice needs review */}
                  {viceNeedsReview && (
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-[var(--text)] w-20">Vice:</span>
                      <button
                        type="button"
                        onClick={() => handleOverride(evaluation, 'vice', 'YES_RISK')}
                        disabled={isProcessing}
                        className={clsx(
                          'rounded-lg px-3 py-1.5 text-sm font-medium transition-all',
                          'bg-red-100 text-red-700 hover:bg-red-200',
                          'disabled:opacity-50 disabled:cursor-not-allowed'
                        )}
                      >
                        Yes
                      </button>
                      <button
                        type="button"
                        onClick={() => handleOverride(evaluation, 'vice', 'NO_RISK')}
                        disabled={isProcessing}
                        className={clsx(
                          'rounded-lg px-3 py-1.5 text-sm font-medium transition-all',
                          'bg-green-100 text-green-700 hover:bg-green-200',
                          'disabled:opacity-50 disabled:cursor-not-allowed'
                        )}
                      >
                        No
                      </button>
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
