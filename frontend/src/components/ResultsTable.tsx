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

const SORT_OPTIONS = [
  { value: 'created_desc', label: 'Newest first' },
  { value: 'created_asc', label: 'Oldest first' },
  { value: 'domain_asc', label: 'Domain A–Z' },
  { value: 'domain_desc', label: 'Domain Z–A' },
  { value: 'trademark_desc', label: 'Trademark high → low' },
  { value: 'trademark_asc', label: 'Trademark low → high' },
  { value: 'vice_desc', label: 'Vice high → low' },
  { value: 'vice_asc', label: 'Vice low → high' }
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
  batchName
}: ResultsTableProps) {
  const [search, setSearch] = useState('');
  const [minScore, setMinScore] = useState<number | undefined>(undefined);
  const [minViceScore, setMinViceScore] = useState<number | undefined>(undefined);
  const [tld, setTld] = useState('');
  const [recommendation, setRecommendation] = useState<string | undefined>(undefined);
  const [sort, setSort] = useState<string>(filters.sort ?? 'created_desc');

  const safeTldOptions = Array.isArray(tldOptions) ? tldOptions : [];
  const rows = Array.isArray(data) ? data : [];

  useEffect(() => {
    const handle = window.setTimeout(() => {
      onQueryChange({
        q: search || undefined,
        minScore,
        minViceScore,
        tld: tld.trim() ? tld.trim() : undefined,
        recommendation,
        sort
      });
    }, 220);
    return () => window.clearTimeout(handle);
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
    <section className="rounded-2xl border border-slate-800 bg-slate-900/70 shadow-sm">
      <div className="space-y-5 p-5">
        <header className="flex flex-col gap-1">
          <h2 className="text-base font-semibold text-slate-100">Evaluation results</h2>
          <p className="text-sm text-slate-400">
            {batchName ? `${batchName} • ` : ''}
            {total.toLocaleString()} domains scored.
          </p>
        </header>

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <input
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search domain or trademark"
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:border-slate-200 focus:outline-none"
          />
          <select
            value={minScore ?? ''}
            onChange={(event) => {
              const value = event.target.value;
              setMinScore(value === '' ? undefined : Number(value));
            }}
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 focus:border-slate-200 focus:outline-none"
          >
            <option value="">All trademark scores</option>
            <option value="5">Trademark ≥ 5</option>
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
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 focus:border-slate-200 focus:outline-none"
          >
            <option value="">All vice scores</option>
            <option value="5">Vice ≥ 5</option>
            <option value="4">Vice ≥ 4</option>
            <option value="3">Vice ≥ 3</option>
          </select>
          <select
            value={recommendation ?? ''}
            onChange={(event) => {
              const value = event.target.value;
              setRecommendation(value === '' ? undefined : value);
            }}
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 focus:border-slate-200 focus:outline-none"
          >
            <option value="">All recommendations</option>
            <option value="BLOCK">Block</option>
            <option value="REVIEW">Review</option>
            <option value="ALLOW_WITH_CAUTION">Allow with caution</option>
            <option value="ALLOW">Allow</option>
          </select>
          <select
            value={sort}
            onChange={(event) => setSort(event.target.value)}
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 focus:border-slate-200 focus:outline-none"
          >
            {SORT_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <select
            value={tld}
            onChange={(event) => setTld(event.target.value)}
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 focus:border-slate-200 focus:outline-none"
          >
            <option value="">All TLDs</option>
            {safeTldOptions.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
          <div className="flex items-center gap-2 text-xs text-slate-400 lg:col-span-2">
            <span>
              Page {page + 1} of {totalPages}
            </span>
            <span className="hidden sm:inline">•</span>
            <ExportButtons onExport={onExport} disabled={loading || rows.length === 0} />
          </div>
        </div>
      </div>

      <div className="overflow-x-auto border-t border-slate-800">
        <table className="w-full min-w-full table-fixed divide-y divide-slate-800 text-sm">
          <thead className="bg-slate-900/40 text-xs uppercase tracking-wide text-slate-400">
            <tr>
              <th className="w-[240px] px-6 py-4 text-left">Domain</th>
              <th className="w-[180px] px-6 py-4 text-left">Trademark</th>
              <th className="w-[220px] px-6 py-4 text-left">Matched mark</th>
              <th className="w-[220px] px-6 py-4 text-left">Vice</th>
              <th className="w-[180px] px-6 py-4 text-left">Recommendation</th>
              <th className="px-6 py-4 text-left md:w-[560px] xl:w-[720px]">Explanation</th>
              <th className="w-[120px] px-6 py-4 text-right">Confidence</th>
              <th className="w-[220px] px-6 py-4 text-left">Commercial</th>
              <th className="w-[160px] px-6 py-4 text-right">Evaluated</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800">
            {loading ? (
              <tr>
                <td colSpan={9} className="px-6 py-12 text-center text-slate-400">
                  Processing…
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={9} className="px-6 py-12 text-center text-slate-400">
                  No results yet. Upload a CSV and run an evaluation.
                </td>
              </tr>
            ) : (
              rows.map((row) => (
                <tr key={row.id} className="bg-slate-900/70 hover:bg-slate-900/80">
                  <td className="px-6 py-5 align-top font-medium text-slate-100 whitespace-nowrap overflow-hidden text-ellipsis">{row.domain}</td>
                  <td className="px-6 py-5 align-top">
                    <div className="flex items-center gap-2">
                      <ScoreBadge variant="trademark" score={row.trademark_score} label={row.trademark_score} />
                      <span className="text-xs text-slate-400">{row.trademark_type || '—'}</span>
                    </div>
                  </td>
                  <td className="px-6 py-5 align-top text-slate-300 break-words">{row.matched_trademark || '—'}</td>
                  <td className="px-6 py-5 align-top">
                    <div className="flex items-center gap-2">
                      <ScoreBadge variant="vice" score={row.vice_score} label={row.vice_score} />
                      <span className="text-xs text-slate-400">
                        {(row.vice_categories ?? []).join(', ') || '—'}
                      </span>
                    </div>
                  </td>
                  <td className="px-6 py-5 align-top">
                    <ScoreBadge variant="overall" label={row.overall_recommendation} />
                  </td>
                  <td className="px-6 py-5 align-top whitespace-pre-line text-slate-200 md:w-[560px] xl:w-[720px] leading-relaxed">
                    {row.explanation || '—'}
                  </td>
                  <td className="px-6 py-5 align-top text-right text-slate-300">
                    {row.confidence.toFixed(2)}
                  </td>
                  <td className="px-6 py-5 align-top text-xs text-slate-400">
                    {row.commercial_override
                      ? `Override — ${row.commercial_source || 'High sale'} (${Math.round(row.commercial_similarity * 100)}% match)`
                      : row.commercial_source
                        ? `Signal — ${row.commercial_source} (${Math.round(row.commercial_similarity * 100)}% match)`
                        : '—'}
                  </td>
                  <td className="px-6 py-5 text-right text-xs text-slate-400">
                    {dayjs(row.created_at).format('YYYY-MM-DD HH:mm')}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <footer className="flex flex-col gap-3 border-t border-slate-800 px-5 py-4 text-sm text-slate-300 sm:flex-row sm:items-center sm:justify-between">
        <span>
          Page {page + 1} of {totalPages}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => onPageChange(Math.max(0, page - 1))}
            disabled={page === 0 || loading}
            className={clsx(
              'rounded-full px-3 py-1 transition-colors',
              page === 0 || loading
                ? 'cursor-not-allowed bg-slate-800 text-slate-500'
                : 'bg-slate-100 text-slate-900 hover:bg-white'
            )}
          >
            Previous
          </button>
          <button
            type="button"
            onClick={() => onPageChange(Math.min(totalPages - 1, page + 1))}
            disabled={page + 1 >= totalPages || loading}
            className={clsx(
              'rounded-full px-3 py-1 transition-colors',
              page + 1 >= totalPages || loading
                ? 'cursor-not-allowed bg-slate-800 text-slate-500'
                : 'bg-slate-100 text-slate-900 hover:bg-white'
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
          'rounded-full px-3 py-1.5 text-xs font-medium transition-colors',
          disabled ? 'cursor-not-allowed bg-slate-800 text-slate-500' : 'bg-slate-100 text-slate-900 hover:bg-white'
        )}
      >
        Export CSV
      </button>
      <button
        type="button"
        disabled={disabled}
        onClick={() => onExport('json')}
        className={clsx(
          'rounded-full px-3 py-1.5 text-xs font-medium transition-colors',
          disabled ? 'cursor-not-allowed bg-slate-800 text-slate-500' : 'bg-slate-100 text-slate-900 hover:bg-white'
        )}
      >
        Export JSON
      </button>
    </div>
  );
}
