import { Fragment, useEffect, useMemo, useState } from 'react';
import clsx from 'clsx';
import dayjs from 'dayjs';
import ScoreBadge from './ScoreBadge';
import type { EvaluationDTO } from '../types';

interface ResultsTableProps {
  data: EvaluationDTO[];
  total: number;
  loading: boolean;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onQueryChange: (query: {
    q?: string;
    tld?: string;
    recommendation?: string;
    sort?: string;
  }) => void;
  onExport: (format: 'csv' | 'json') => Promise<void>;
  tldOptions: string[];
  filters: {
    q?: string;
    tld?: string;
    recommendation?: string;
    sort?: string;
  };
  batchName?: string;
  onRowClick?: (evaluation: EvaluationDTO) => void;
}

const SORT_OPTIONS = [
  { value: 'created_desc', label: 'Newest first' },
  { value: 'created_asc', label: 'Oldest first' },
  { value: 'domain_asc', label: 'Domain A–Z' },
  { value: 'domain_desc', label: 'Domain Z–A' }
];

const RECOMMENDATION_OPTIONS = [
  { value: '', label: 'All decisions' },
  { value: 'YES_RISK', label: 'Yes Risk' },
  { value: 'POTENTIAL_RISK', label: 'Potential Risk' },
  { value: 'NO_RISK', label: 'No Risk' }
];

export default function ResultsTable({
  data,
  total,
  loading,
  page,
  pageSize,
  onPageChange,
  onQueryChange,
  onExport,
  tldOptions,
  filters,
  batchName,
  onRowClick
}: ResultsTableProps) {
  const [search, setSearch] = useState('');
  const [tld, setTld] = useState('');
  const [recommendation, setRecommendation] = useState<string | undefined>(undefined);
  const [sort, setSort] = useState<string>(filters.sort ?? 'created_desc');
  const [showFilters, setShowFilters] = useState(false);
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());

  const safeTldOptions = Array.isArray(tldOptions) ? tldOptions : [];
  const rows = Array.isArray(data) ? data : [];

  const toggleRow = (id: number, e: React.MouseEvent) => {
    e.stopPropagation();
    setExpandedRows(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  useEffect(() => {
    const handle = window.setTimeout(() => {
      onQueryChange({
        q: search || undefined,
        tld: tld.trim() ? tld.trim() : undefined,
        recommendation,
        sort
      });
    }, 220);
    return () => window.clearTimeout(handle);
  }, [search, tld, recommendation, sort, onQueryChange]);

  useEffect(() => {
    setSearch(filters.q ?? '');
    setTld(filters.tld ?? '');
    setRecommendation(filters.recommendation);
    setSort(filters.sort ?? 'created_desc');
  }, [filters.q, filters.tld, filters.recommendation, filters.sort]);

  const totalPages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total, pageSize]);
  const hasActiveFilters = tld !== '' || recommendation !== undefined;

  return (
    <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] overflow-hidden">
      {/* Header */}
      <div className="p-5 border-b border-[var(--line)]">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-[var(--text)]">Results</h2>
            <p className="text-sm text-[var(--muted)]">
              {batchName ? `${batchName} · ` : ''}{total.toLocaleString()} domains
            </p>
          </div>
          <ExportButtons onExport={onExport} disabled={loading || rows.length === 0} />
        </div>

        {/* Search and primary filters */}
        <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center">
          <div className="flex-1">
            <input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search domain..."
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-4 py-2.5 text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
            />
          </div>
          <select
            value={recommendation ?? ''}
            onChange={(event) => {
              const value = event.target.value;
              setRecommendation(value === '' ? undefined : value);
            }}
            className="rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-4 py-2.5 text-[var(--text)] focus:border-[var(--accent)] focus:outline-none"
          >
            {RECOMMENDATION_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <select
            value={sort}
            onChange={(event) => setSort(event.target.value)}
            className="rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-4 py-2.5 text-[var(--text)] focus:border-[var(--accent)] focus:outline-none"
          >
            {SORT_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            onClick={() => setShowFilters(!showFilters)}
            className={clsx(
              'rounded-lg border px-4 py-2.5 text-sm font-medium flex items-center gap-2 whitespace-nowrap',
              showFilters || hasActiveFilters
                ? 'border-[var(--accent)] text-[var(--accent)] bg-[var(--accent-soft)]'
                : 'border-[var(--line)] text-[var(--text)] hover:bg-[var(--surface-2)]'
            )}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
            </svg>
            Filters
            {hasActiveFilters && <span className="w-2 h-2 rounded-full bg-[var(--accent)]" />}
          </button>
        </div>

        {/* Advanced filters (collapsible) */}
        {showFilters && (
          <div className="mt-4 p-4 bg-[var(--surface-2)] rounded-lg border border-[var(--line)]">
            <div className="flex flex-wrap gap-4 items-end">
              <div>
                <label className="block text-sm font-medium text-[var(--muted)] mb-1">TLD</label>
                <select
                  value={tld}
                  onChange={(event) => setTld(event.target.value)}
                  className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2 text-[var(--text)]"
                >
                  <option value="">All TLDs</option>
                  {safeTldOptions.map((option) => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </div>
              <button
                type="button"
                onClick={() => {
                  setTld('');
                  setRecommendation(undefined);
                }}
                className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm font-medium text-[var(--muted)] hover:bg-[var(--surface)]"
              >
                Clear all
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Table - Bifurcated: Domain, Trademark, Vice, Date */}
      <div className="overflow-x-auto">
        <table className="w-full text-left">
          <thead className="bg-[var(--surface-2)] text-sm text-[var(--muted)] border-b border-[var(--line)]">
            <tr>
              <th className="w-10 px-4 py-3"></th>
              <th className="px-4 py-3 font-medium">Domain</th>
              <th className="px-4 py-3 font-medium w-36">Trademark</th>
              <th className="px-4 py-3 font-medium w-36">Vice</th>
              <th className="px-4 py-3 font-medium text-right w-32">Date</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--line)]">
            {loading ? (
              <tr>
                <td colSpan={5} className="px-4 py-12 text-center text-[var(--muted)]">
                  <div className="flex items-center justify-center gap-2">
                    <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                    </svg>
                    Loading...
                  </div>
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={5} className="px-4 py-12 text-center text-[var(--muted)]">
                  No results yet. Upload a CSV and run an evaluation.
                </td>
              </tr>
            ) : (
              rows.map((row) => {
                const isExpanded = expandedRows.has(row.id);
                return (
                  <Fragment key={row.id}>
                  <tr
                      className={clsx(
                        'transition-colors',
                        isExpanded ? 'bg-[var(--accent-soft)]' : 'hover:bg-[var(--surface-2)]',
                        onRowClick && 'cursor-pointer'
                      )}
                      onClick={() => onRowClick?.(row)}
                    >
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          onClick={(e) => toggleRow(row.id, e)}
                          className="p-1 rounded hover:bg-[var(--surface-2)]"
                        >
                          <svg
                            className={clsx('w-4 h-4 text-[var(--muted)] transition-transform', isExpanded && 'rotate-90')}
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                          >
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                          </svg>
                        </button>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-[var(--text)]">{row.domain}</span>
                          {row.manual_override && (
                            <span className="inline-flex items-center rounded bg-purple-100 px-1.5 py-0.5 text-xs font-medium text-purple-700">
                              Override{row.override_count > 1 && ` (${row.override_count})`}
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <ScoreBadge label={row.trademark_recommendation} />
                      </td>
                      <td className="px-4 py-3">
                        <ScoreBadge label={row.vice_recommendation} />
                      </td>
                      <td className="px-4 py-3 text-right text-sm text-[var(--muted)]">
                        {dayjs(row.created_at).format('MMM D, HH:mm')}
                      </td>
                    </tr>
                    {isExpanded && (
                      <tr className="bg-[var(--surface-2)]">
                        <td colSpan={5} className="px-4 py-4">
                          {/* AI Explanation - Main focus */}
                          {row.explanation && (
                            <div className="bg-[var(--surface)] rounded-lg p-4 border border-[var(--line)]">
                              <h4 className="font-medium text-[var(--text)] mb-2">AI Explanation</h4>
                              <p className="text-sm text-[var(--text)] leading-relaxed">{row.explanation}</p>
                            </div>
                          )}

                          {/* Additional Details */}
                          <div className="mt-4 grid gap-4 md:grid-cols-3">
                            {/* Trademark Analysis */}
                            <div className="bg-[var(--surface)] rounded-lg p-4 border border-[var(--line)]">
                              <h4 className="font-medium text-[var(--text)] mb-2">Trademark Analysis</h4>
                              <div className="space-y-1 text-sm">
                                <p><span className="text-[var(--muted)]">Type:</span> <span className="text-[var(--text)]">{row.trademark_type || 'None'}</span></p>
                                {row.matched_trademark && (
                                  <p><span className="text-[var(--muted)]">Matched:</span> <span className="text-[var(--text)]">{row.matched_trademark}</span></p>
                                )}
                                <p><span className="text-[var(--muted)]">Confidence:</span> <span className="text-[var(--text)]">{(row.trademark_confidence * 100).toFixed(0)}%</span></p>
                              </div>
                            </div>

                            {/* Vice Analysis */}
                            <div className="bg-[var(--surface)] rounded-lg p-4 border border-[var(--line)]">
                              <h4 className="font-medium text-[var(--text)] mb-2">Vice Analysis</h4>
                              <div className="space-y-1 text-sm">
                                {row.vice_categories && row.vice_categories.length > 0 ? (
                                  <p><span className="text-[var(--muted)]">Categories:</span> <span className="text-[var(--text)]">{row.vice_categories.join(', ')}</span></p>
                                ) : (
                                  <p><span className="text-[var(--muted)]">Categories:</span> <span className="text-[var(--text)]">None</span></p>
                                )}
                                <p><span className="text-[var(--muted)]">Confidence:</span> <span className="text-[var(--text)]">{(row.vice_confidence * 100).toFixed(0)}%</span></p>
                              </div>
                            </div>

                            {/* Commercial Signal */}
                            <div className="bg-[var(--surface)] rounded-lg p-4 border border-[var(--line)]">
                              <h4 className="font-medium text-[var(--text)] mb-2">Commercial Signal</h4>
                              <div className="space-y-1 text-sm">
                                {row.commercial_source ? (
                                  <>
                                    <p><span className="text-[var(--muted)]">Source:</span> <span className="text-[var(--text)]">{row.commercial_source}</span></p>
                                    <p><span className="text-[var(--muted)]">Similarity:</span> <span className="text-[var(--text)]">{(row.commercial_similarity * 100).toFixed(0)}%</span></p>
                                  </>
                                ) : (
                                  <p className="text-[var(--muted)]">No commercial data</p>
                                )}
                              </div>
                            </div>
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Footer / Pagination */}
      <footer className="flex flex-col gap-3 border-t border-[var(--line)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <span className="text-sm text-[var(--muted)]">
          Page {page + 1} of {totalPages} · {total.toLocaleString()} total
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => onPageChange(Math.max(0, page - 1))}
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
            onClick={() => onPageChange(Math.min(totalPages - 1, page + 1))}
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
      </footer>
    </section>
  );
}

function ExportButtons({ onExport, disabled }: { onExport: (format: 'csv' | 'json') => Promise<void>; disabled: boolean }) {
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        disabled={disabled}
        onClick={() => onExport('csv')}
        className={clsx(
          'rounded-lg border border-[var(--line)] px-3 py-2 text-sm font-medium text-[var(--text)]',
          disabled ? 'cursor-not-allowed opacity-40' : 'hover:bg-[var(--surface-2)]'
        )}
      >
        CSV
      </button>
      <button
        type="button"
        disabled={disabled}
        onClick={() => onExport('json')}
        className={clsx(
          'rounded-lg border border-[var(--line)] px-3 py-2 text-sm font-medium text-[var(--text)]',
          disabled ? 'cursor-not-allowed opacity-40' : 'hover:bg-[var(--surface-2)]'
        )}
      >
        JSON
      </button>
    </div>
  );
}
