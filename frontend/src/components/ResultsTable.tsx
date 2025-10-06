import { useEffect, useMemo, useState } from 'react';
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
    minScore?: number;
    minViceScore?: number;
    tld?: string;
    recommendation?: string;
    sort?: string;
  }) => void;
  onExport: (format: 'csv' | 'json') => Promise<void>;
  tldOptions: string[];
  filters: {
    q?: string;
    minScore?: number;
    minViceScore?: number;
    tld?: string;
    recommendation?: string;
    sort?: string;
  };
  batchName?: string;
}

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
  batchName
}: ResultsTableProps) {
  console.log('[ResultsTable] Render - data:', data?.length, 'total:', total, 'loading:', loading, 'batchName:', batchName);

  const [search, setSearch] = useState('');
  const [minScore, setMinScore] = useState<number | undefined>(undefined);
  const [minViceScore, setMinViceScore] = useState<number | undefined>(undefined);
  const [tld, setTld] = useState('');
  const [recommendation, setRecommendation] = useState<string | undefined>(undefined);
  const [sort, setSort] = useState<string>(filters.sort ?? 'created_desc');

  const safeTldOptions = Array.isArray(tldOptions) ? tldOptions : [];
  const rows = Array.isArray(data) ? data : [];

  console.log('[ResultsTable] Rows to display:', rows.length, 'first row:', rows[0]);

  useEffect(() => {
    const handle = setTimeout(() => {
      onQueryChange({
        q: search || undefined,
        minScore,
        minViceScore,
        tld: tld.trim() ? tld.trim() : undefined,
        recommendation,
        sort
      });
    }, 250);
    return () => clearTimeout(handle);
  }, [search, minScore, minViceScore, tld, recommendation, sort, onQueryChange]);

  useEffect(() => {
    setSearch(filters.q ?? '');
    setMinScore(filters.minScore);
    setMinViceScore(filters.minViceScore);
    setTld(filters.tld ?? '');
    setRecommendation(filters.recommendation);
    setSort(filters.sort ?? 'created_desc');
  }, [filters.q, filters.minScore, filters.minViceScore, filters.tld, filters.recommendation, filters.sort]);

  const totalPages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total, pageSize]);

  return (
    <section className="bg-slate-900 border border-slate-800 rounded-xl shadow-lg">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between p-6 border-b border-slate-800">
        <div>
          <h2 className="text-lg font-semibold">
            Evaluation Results
            {batchName ? <span className="text-slate-400"> • {batchName}</span> : null}
          </h2>
          <p className="text-sm text-slate-400">{total.toLocaleString()} domains evaluated.</p>
        </div>
        <div className="flex flex-wrap gap-3">
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search domain or mark"
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          />
          <select
            value={minScore ?? ''}
            onChange={(event) => {
              const value = event.target.value;
              setMinScore(value === '' ? undefined : Number(value));
            }}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          >
            <option value="">All scores</option>
            <option value="4">Trademark ≥ 4</option>
            <option value="3">Trademark ≥ 3</option>
            <option value="2">Trademark ≥ 2</option>
          </select>
          <select
            value={minViceScore ?? ''}
            onChange={(event) => {
              const value = event.target.value;
              setMinViceScore(value === '' ? undefined : Number(value));
            }}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          >
            <option value="">All vice scores</option>
            <option value="5">Vice ≥ 5</option>
            <option value="4">Vice ≥ 4</option>
            <option value="3">Vice ≥ 3</option>
            <option value="2">Vice ≥ 2</option>
          </select>
          <select
            value={tld}
            onChange={(event) => setTld(event.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          >
            <option value="">All TLDs</option>
            {safeTldOptions.map((option) => (
              <option key={option} value={option}>
                .{option}
              </option>
            ))}
          </select>
          <select
            value={recommendation ?? ''}
            onChange={(event) => {
              const value = event.target.value;
              setRecommendation(value === '' ? undefined : value);
            }}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          >
            <option value="">All recommendations</option>
            <option value="BLOCK">Block</option>
            <option value="REVIEW">Review</option>
            <option value="ALLOW_WITH_CAUTION">Allow w/ Caution</option>
            <option value="ALLOW">Allow</option>
          </select>
          <select
            value={sort}
            onChange={(event) => setSort(event.target.value)}
            className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-500"
          >
            <option value="created_desc">Newest first</option>
            <option value="created_asc">Oldest first</option>
            <option value="domain_asc">Domain A-Z</option>
            <option value="domain_desc">Domain Z-A</option>
            <option value="trademark_desc">Trademark score (high → low)</option>
            <option value="trademark_asc">Trademark score (low → high)</option>
            <option value="vice_desc">Vice score (high → low)</option>
            <option value="vice_asc">Vice score (low → high)</option>
          </select>
          <ExportButtons onExport={onExport} disabled={loading} />
        </div>
      </header>

      <div className="overflow-x-auto">
        <table className="min-w-full text-sm">
          <thead className="bg-slate-800/60 uppercase text-xs text-slate-400">
            <tr>
              <th className="px-4 py-3 text-left">Domain</th>
              <th className="px-4 py-3 text-left">Trademark Score</th>
              <th className="px-4 py-3 text-left">Trademark Type</th>
              <th className="px-4 py-3 text-left">Matched Trademark</th>
              <th className="px-4 py-3 text-left">Vice Score</th>
              <th className="px-4 py-3 text-left">Vice Categories</th>
              <th className="px-4 py-3 text-left">Recommendation</th>
              <th className="px-4 py-3 text-left w-2/5">AI Explanation</th>
              <th className="px-4 py-3 text-right">Confidence</th>
              <th className="px-4 py-3 text-left">Commercial</th>
              <th className="px-4 py-3 text-right">Evaluated</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan={11} className="px-4 py-8 text-center text-slate-400">
                  Processing…
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={11} className="px-4 py-8 text-center text-slate-400">
                  No results yet. Upload files and run an evaluation.
                </td>
              </tr>
            ) : (
              rows.map((row) => (
                <tr key={row.id} className="odd:bg-slate-900 even:bg-slate-900/60">
                  <td className="px-4 py-3 font-medium text-slate-100">{row.domain}</td>
                  <td className="px-4 py-3">
                    <ScoreBadge variant="trademark" score={row.trademark_score} label={row.trademark_score} />
                  </td>
                  <td className="px-4 py-3 text-slate-300">{row.trademark_type || '—'}</td>
                  <td className="px-4 py-3 text-slate-200">{row.matched_trademark || '—'}</td>
                  <td className="px-4 py-3">
                    <ScoreBadge variant="vice" score={row.vice_score} label={row.vice_score} />
                  </td>
                  <td className="px-4 py-3 text-slate-300">{(row.vice_categories ?? []).join(', ') || '—'}</td>
                  <td className="px-4 py-3">
                    <ScoreBadge variant="overall" label={row.overall_recommendation} />
                  </td>
                  <td className="px-4 py-3 text-slate-300 whitespace-pre-line align-top min-w-[360px]">
                    {row.explanation || '—'}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {row.confidence.toFixed(2)}
                  </td>
                  <td className="px-4 py-3 text-left text-xs">
                    {row.commercial_override
                      ? `Override — ${row.commercial_source || 'high-value sale'} (${Math.round(row.commercial_similarity * 100)}% match)`
                      : row.commercial_source
                        ? `Signal — ${row.commercial_source} (${Math.round(row.commercial_similarity * 100)}% match)`
                        : 'No'}
                  </td>
                  <td className="px-4 py-3 text-right text-xs text-slate-400">
                    {dayjs(row.created_at).format('YYYY-MM-DD HH:mm:ss')}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <footer className="flex flex-col sm:flex-row items-center justify-between gap-4 px-6 py-4 border-t border-slate-800 text-sm">
        <span className="text-slate-400">
          Page {page + 1} of {totalPages}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => onPageChange(Math.max(0, page - 1))}
            disabled={page === 0 || loading}
            className={clsx('px-3 py-1 rounded-md border border-slate-700',
              page === 0 || loading ? 'text-slate-500 cursor-not-allowed' : 'text-slate-200 hover:bg-slate-800')}
          >
            Previous
          </button>
          <button
            type="button"
            onClick={() => onPageChange(Math.min(totalPages - 1, page + 1))}
            disabled={page + 1 >= totalPages || loading}
            className={clsx('px-3 py-1 rounded-md border border-slate-700',
              page + 1 >= totalPages || loading ? 'text-slate-500 cursor-not-allowed' : 'text-slate-200 hover:bg-slate-800')}
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
    <div className="flex gap-2">
      <button
        type="button"
        disabled={disabled}
        onClick={() => onExport('csv')}
        className={clsx(
          'px-3 py-2 rounded-lg text-sm border border-slate-700 hover:bg-slate-800 transition-colors',
          disabled && 'opacity-50 cursor-not-allowed'
        )}
      >
        Export CSV
      </button>
      <button
        type="button"
        disabled={disabled}
        onClick={() => onExport('json')}
        className={clsx(
          'px-3 py-2 rounded-lg text-sm border border-slate-700 hover:bg-slate-800 transition-colors',
          disabled && 'opacity-50 cursor-not-allowed'
        )}
      >
        Export JSON
      </button>
    </div>
  );
}
