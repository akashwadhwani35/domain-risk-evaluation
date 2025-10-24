import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import clsx from 'clsx';
import { isAxiosError } from 'axios';
import UploadPane from './components/UploadPane';
import ResultsTable from './components/ResultsTable';
import CsvExplorer from './components/CsvExplorer';
import {
  buildWebSocketURL,
  cancelEvaluation,
  fetchConfig,
  exportResults,
  fetchResults,
  mapEvaluation,
  triggerEvaluation,
  uploadFiles,
  listBatches,
  fetchBatch,
  fetchEvaluationStatus
} from './lib/api';
import type { BatchDTO, EvaluationDTO, EvaluationEvent, StartEvaluationResponse, EvaluateRequest, UploadResponse } from './types';

const PAGE_SIZE = 50;
const DEFAULT_SORT = 'created_desc';

type FiltersState = {
  q?: string;
  minScore?: number;
  minViceScore?: number;
  tld?: string;
  recommendation?: string;
  sort?: string;
};

type ProgressState = {
  status: 'idle' | 'running' | 'cancelling' | 'complete' | 'error' | 'cancelled';
  processed: number;
  total: number;
  message?: string;
};

type EvaluateOptions = {
  batchId?: number;
  resume?: boolean;
  force?: boolean;
};

const normalizeSort = (sort?: string) => (sort && sort.trim() !== '' ? sort : DEFAULT_SORT);

const matchesFilters = (row: EvaluationDTO, filters: FiltersState) => {
  if (typeof filters.minScore === 'number' && row.trademark_score < filters.minScore) {
    return false;
  }
  if (typeof filters.minViceScore === 'number' && row.vice_score < filters.minViceScore) {
    return false;
  }
  if (filters.tld) {
    const tld = filters.tld.startsWith('.') ? filters.tld.toLowerCase() : `.${filters.tld.toLowerCase()}`;
    if (!row.domain.toLowerCase().endsWith(tld)) {
      return false;
    }
  }
  if (filters.recommendation) {
    if (row.overall_recommendation !== filters.recommendation.toUpperCase()) {
      return false;
    }
  }
  if (filters.q) {
    const query = filters.q.toLowerCase();
    const domainMatch = row.domain.toLowerCase().includes(query);
    const mark = (row.matched_trademark ?? '').toLowerCase();
    const markMatch = mark.includes(query);
    if (!domainMatch && !markMatch) {
      return false;
    }
  }
  return true;
};

const sortEvaluations = (rows: EvaluationDTO[], sort?: string) => {
  const order = normalizeSort(sort);
  const next = [...rows];
  switch (order) {
    case 'domain_asc':
      next.sort((a, b) => a.domain.localeCompare(b.domain));
      break;
    case 'domain_desc':
      next.sort((a, b) => b.domain.localeCompare(a.domain));
      break;
    case 'trademark_desc':
      next.sort((a, b) => {
        if (b.trademark_score !== a.trademark_score) return b.trademark_score - a.trademark_score;
        return b.vice_score - a.vice_score;
      });
      break;
    case 'trademark_asc':
      next.sort((a, b) => {
        if (a.trademark_score !== b.trademark_score) return a.trademark_score - b.trademark_score;
        return a.vice_score - b.vice_score;
      });
      break;
    case 'vice_desc':
      next.sort((a, b) => {
        if (b.vice_score !== a.vice_score) return b.vice_score - a.vice_score;
        return b.trademark_score - a.trademark_score;
      });
      break;
    case 'vice_asc':
      next.sort((a, b) => {
        if (a.vice_score !== b.vice_score) return a.vice_score - b.vice_score;
        return a.trademark_score - b.trademark_score;
      });
      break;
    case 'created_asc':
      next.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
      break;
    case 'created_desc':
    default:
      next.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
      break;
  }
  return next;
};

const applyEvaluationUpdate = (rows: EvaluationDTO[], evaluation: EvaluationDTO, sort?: string) => {
  const filtered = rows.filter((row) => row.id !== evaluation.id);
  filtered.push(evaluation);
  return sortEvaluations(filtered, sort);
};

export default function App() {
  const [evaluations, setEvaluations] = useState<EvaluationDTO[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [filters, setFilters] = useState<FiltersState>({ sort: DEFAULT_SORT });
  const [job, setJob] = useState<StartEvaluationResponse | null>(null);
  const [progress, setProgress] = useState<ProgressState>({ status: 'idle', processed: 0, total: 0, message: '' });
  const [liveNotice, setLiveNotice] = useState(false);
  const [tldOptions, setTldOptions] = useState<string[]>([]);
  const [cancelling, setCancelling] = useState(false);
  const [batches, setBatches] = useState<BatchDTO[]>([]);
  const [batchLoading, setBatchLoading] = useState(false);
  const [selectedBatch, setSelectedBatch] = useState<BatchDTO | null>(null);
  const [explorerOpen, setExplorerOpen] = useState(false);
  const [liveEvaluations, setLiveEvaluations] = useState<EvaluationDTO[]>([]);
  const [evaluationMessage, setEvaluationMessage] = useState<string | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const filtersRef = useRef(filters);
  const pageRef = useRef(page);
  const loadResultsRef = useRef<() => Promise<void>>();
  const selectedBatchIdRef = useRef<number | null>(null);

  const isEvaluating = Boolean(
    job && (progress.status === 'running' || progress.status === 'cancelling')
  );

  const filterQuery = filters.q;
  const filterMinScore = filters.minScore;
  const filterMinViceScore = filters.minViceScore;
  const filterTld = filters.tld;
  const filterRecommendation = filters.recommendation;
  const filterSort = normalizeSort(filters.sort);

  const loadResults = useCallback(async () => {
    setLoading(true);
    try {
      const batchId = selectedBatch?.id;
      const response = await fetchResults({
        q: filterQuery,
        minScore: filterMinScore,
        minViceScore: filterMinViceScore,
        tld: filterTld,
        recommendation: filterRecommendation,
        sort: filterSort,
        page,
        pageSize: PAGE_SIZE,
        batchId
      });
      const mapped = response.items.map(mapEvaluation);
      setEvaluations(mapped);
      setTotal(response.total);
    } catch (err) {
      console.error('[loadResults] Failed to fetch results', err);
    } finally {
      setLoading(false);
    }
  }, [filterQuery, filterMinScore, filterMinViceScore, filterTld, filterRecommendation, filterSort, page, selectedBatch?.id]);

  loadResultsRef.current = loadResults;

  const touchSelectedBatch = useCallback(
    (updater: (current: BatchDTO) => BatchDTO) => {
      setSelectedBatch((current) => {
        if (!current) return current;
        return updater(current);
      });
    },
    []
  );

  const refreshConfig = useCallback(async () => {
    try {
      const config = await fetchConfig();
      setTldOptions(Array.isArray(config.tlds) ? config.tlds : []);
    } catch (err) {
      console.error('Failed to load config', err);
    }
  }, []);

  const loadBatches = useCallback(async (preferredId?: number) => {
    setBatchLoading(true);
    try {
      const response = await listBatches();
      const next = Array.isArray(response.items) ? response.items : [];
      setBatches(next);

      const fallbackId = preferredId ?? selectedBatchIdRef.current ?? (next.length > 0 ? next[0].id : null);
      if (fallbackId) {
        const match = next.find((batch) => batch.id === fallbackId);
        if (match) {
          setSelectedBatch(match);
        } else {
          const details = await fetchBatch(fallbackId);
          setSelectedBatch(details);
          setBatches((prev) => {
            const filtered = prev.filter((item) => item.id !== details.id);
            return [details, ...filtered];
          });
        }
      } else {
        setSelectedBatch(null);
      }
    } catch (err) {
      console.error('[loadBatches] Failed to load batches', err);
    } finally {
      setBatchLoading(false);
    }
  }, []);

  const syncEvaluationStatus = useCallback(async () => {
    try {
      const status = await fetchEvaluationStatus();
      if (!status.running) {
        setJob(null);
        if (status.state === 'complete') {
          setProgress((prev) => ({ ...prev, status: 'complete', processed: status.processed ?? prev.processed, total: status.total ?? prev.total, message: status.message ?? 'Evaluation complete' }));
        } else if (status.state === 'cancelled') {
          setProgress((prev) => ({ ...prev, status: 'cancelled', processed: status.processed ?? prev.processed, total: status.total ?? prev.total, message: status.message ?? 'Evaluation cancelled' }));
        } else if (status.state === 'error') {
          setProgress((prev) => ({ ...prev, status: 'error', processed: status.processed ?? prev.processed, total: status.total ?? prev.total, message: status.message ?? 'Evaluation failed' }));
        } else {
          setProgress((prev) => ({ ...prev, status: 'idle', processed: 0, total: prev.total, message: status.message ?? prev.message }));
        }
        if (status.message) {
          setEvaluationMessage(status.message);
        }
        return;
      }

      if (!status.job_id) {
        return;
      }

      const info: StartEvaluationResponse = {
        job_id: status.job_id,
        batch_id: status.batch_id ?? selectedBatchIdRef.current ?? 0,
        request_id: status.request_id ?? 0,
        total: status.total ?? 0,
        started_at: new Date().toISOString()
      };
      setJob(info);
      const processed = status.processed ?? 0;
      const total = status.total ?? info.total;
      setProgress({ status: 'running', processed, total, message: status.message ?? 'Evaluation running…' });
      setEvaluationMessage(status.message ?? 'Evaluation running…');

      if (status.last_evaluation) {
        const evaluation = mapEvaluation(status.last_evaluation);
        setLiveEvaluations((prev) => {
          if (prev.some((item) => item.id === evaluation.id)) {
            return prev;
          }
          const next = [evaluation, ...prev];
          return next.slice(0, 20);
        });
      }

      if (status.batch_id && selectedBatchIdRef.current === status.batch_id) {
        touchSelectedBatch((current) => ({
          ...current,
          processed_domains: processed,
          last_evaluated_at: new Date().toISOString()
        }));
      } else if (status.batch_id && selectedBatchIdRef.current === null) {
        selectedBatchIdRef.current = status.batch_id;
      }
    } catch (err) {
      console.error('Failed to sync evaluation status', err);
    }
  }, [touchSelectedBatch]);

  const handleBatchRefresh = useCallback(() => {
    loadBatches(selectedBatchIdRef.current ?? undefined).catch((err) => console.error(err));
  }, [loadBatches]);

  const handleSelectBatch = useCallback((batch: BatchDTO) => {
    setSelectedBatch(batch);
    selectedBatchIdRef.current = batch.id;
    setEvaluationMessage(null);
    setExplorerOpen(false);
    setPage(0);
    setEvaluations([]);
    setTotal(0);
    setLiveEvaluations([]);
    const reload = loadResultsRef.current;
    if (reload) {
      reload().catch((err) => console.error(err));
    }
    fetchBatch(batch.id)
      .then((updated) => {
        setSelectedBatch(updated);
        setBatches((prev) => {
          const filtered = prev.filter((item) => item.id !== updated.id);
          return [updated, ...filtered];
        });
      })
      .catch((err) => console.error(err));
  }, [fetchBatch]);

  const evaluateBatch = useCallback(
    async ({ batchId, resume, force }: EvaluateOptions = {}) => {
      const targetId = batchId ?? selectedBatchIdRef.current ?? selectedBatch?.id ?? null;
      if (!targetId) {
        setEvaluationMessage('Select a dataset before starting an evaluation.');
        return;
      }

      setBusy(true);
      try {
        const payload: EvaluateRequest = { batch_id: targetId };
        if (resume) payload.resume = true;
        if (force) payload.force = true;

        const response = await triggerEvaluation(payload);
        setJob(response);
        setProgress({
          status: 'running',
          processed: 0,
          total: response.total ?? 0,
          message: 'Evaluation queued…'
        });
        setEvaluationMessage('Evaluation started…');
        selectedBatchIdRef.current = targetId;
        await loadBatches(targetId);
      } catch (err) {
        const message =
          isAxiosError(err) && err.response?.data
            ? (err.response.data as { message?: string })?.message ?? 'Evaluation failed'
            : err instanceof Error
              ? err.message
              : 'Evaluation failed';
        setEvaluationMessage(message);
        console.error('Failed to start evaluation', err);
      } finally {
        setBusy(false);
      }
    },
    [loadBatches, selectedBatch?.id]
  );

  const handleEvaluate = useCallback(
    async (options?: EvaluateOptions) => {
      await evaluateBatch(options);
    },
    [evaluateBatch]
  );

  const handleProcess = useCallback(
    async (form: FormData): Promise<UploadResponse> => {
      setBusy(true);
      try {
        const response = await uploadFiles(form);
        setEvaluationMessage(`Uploaded ${response.row_count.toLocaleString()} rows.`);
        await loadBatches(response.batch_id);
        selectedBatchIdRef.current = response.batch_id;
        return response;
      } catch (err) {
        const message =
          err instanceof Error ? err.message : 'Upload failed';
        setEvaluationMessage(message);
        throw err;
      } finally {
        setBusy(false);
      }
    },
    [loadBatches]
  );

  const handleCancel = useCallback(async () => {
    if (!job) return;
    setCancelling(true);
    try {
      await cancelEvaluation(job.job_id);
      setEvaluationMessage('Cancellation requested…');
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to cancel evaluation';
      setEvaluationMessage(message);
      setCancelling(false);
    }
  }, [job]);

  const handleResume = useCallback(async () => {
    await evaluateBatch({ resume: true });
  }, [evaluateBatch]);

  const handlePageChange = useCallback(
    (nextPage: number) => {
      setPage(nextPage);
      const reload = loadResultsRef.current;
      if (reload) {
        reload().catch((err) => console.error(err));
      }
    },
    []
  );

  const handleQueryChange = useCallback(
    (query: {
      q?: string;
      minScore?: number;
      minViceScore?: number;
      tld?: string;
      recommendation?: string;
      sort?: string;
    }) => {
      setFilters((prev) => ({ ...prev, ...query }));
      setPage(0);
      const reload = loadResultsRef.current;
      if (reload) {
        reload().catch((err) => console.error(err));
      }
    },
    []
  );

  const handleExport = useCallback(
    async (format: 'csv' | 'json') => {
      try {
        const blob = await exportResults(format, { batchId: selectedBatch?.id ?? undefined });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = format === 'csv' ? 'domain-risk-results.csv' : 'domain-risk-results.json';
        document.body.appendChild(link);
        link.click();
        link.remove();
        URL.revokeObjectURL(url);
      } catch (err) {
        console.error('Failed to export results', err);
        setEvaluationMessage('Export failed.');
      }
    },
    [selectedBatch?.id]
  );

  useEffect(() => {
    filtersRef.current = filters;
  }, [filters]);

  useEffect(() => {
    pageRef.current = page;
  }, [page]);

  useEffect(() => {
    selectedBatchIdRef.current = selectedBatch?.id ?? null;
  }, [selectedBatch?.id]);

  useEffect(() => {
    loadResults().catch((err) => console.error(err));
  }, [loadResults]);

  useEffect(() => {
    refreshConfig().catch((err) => console.error(err));
  }, [refreshConfig]);

  useEffect(() => {
    loadBatches().catch((err) => console.error(err));
  }, [loadBatches]);

  const summaryCounts = useMemo(() => {
    const base = {
      BLOCK: 0,
      REVIEW: 0,
      ALLOW_WITH_CAUTION: 0,
      ALLOW: 0,
    };
    evaluations.forEach((row) => {
      const key = (row.overall_recommendation ?? '').toUpperCase();
      if (key in base) {
        base[key as keyof typeof base] += 1;
      }
    });
    return base;
  }, [evaluations]);

  const summaryItems = useMemo(
    () => [
      { label: 'Block', value: summaryCounts.BLOCK, accent: 'bg-red-500/20 text-red-200' },
      { label: 'Review', value: summaryCounts.REVIEW, accent: 'bg-sky-500/20 text-sky-200' },
      { label: 'Caution', value: summaryCounts.ALLOW_WITH_CAUTION, accent: 'bg-amber-500/20 text-amber-200' },
      { label: 'Allow', value: summaryCounts.ALLOW, accent: 'bg-emerald-500/20 text-emerald-200' },
    ],
    [summaryCounts]
  );

  const progressPercent = useMemo(() => {
    if (!progress.total) return 0;
    return Math.min(100, Math.round((progress.processed / progress.total) * 100));
  }, [progress.processed, progress.total]);

  const liveSlice = useMemo(() => liveEvaluations.slice(0, 6), [liveEvaluations]);
  const recentBatches = useMemo(() => batches.slice(0, 6), [batches]);
  const remainingForSelected = selectedBatch ? Math.max(selectedBatch.unique_domains - selectedBatch.processed_domains, 0) : 0;
  const progressNote = evaluationMessage ?? progress.message ?? '';

  useEffect(() => {
    syncEvaluationStatus().catch((err) => console.error('Failed to sync evaluation status', err));
  }, [syncEvaluationStatus]);

  useEffect(() => {
    if (!job) {
      return undefined;
    }

    const baseUrl = buildWebSocketURL('./evaluate/stream');
    const url = new URL(baseUrl);
    url.searchParams.set('jobId', job.job_id);

    const socket = new WebSocket(url.toString());
    socketRef.current = socket;

    socket.onopen = () => {
      setProgress({ status: 'running', processed: 0, total: job.total, message: 'Evaluation started' });
      if (pageRef.current === 0) {
        setEvaluations([]);
      }
      setLiveNotice(false);
      setCancelling(false);
    };

    socket.onerror = (event) => {
      console.error('Evaluation stream error', event);
    };

    socket.onclose = () => {
      socketRef.current = null;
    };

    socket.onmessage = (event) => {
      try {
        const payload: EvaluationEvent = JSON.parse(event.data);
        if (!payload || payload.job_id !== job.job_id) {
          return;
        }

        if (job.batch_id && payload.batch_id && payload.batch_id !== job.batch_id) {
          return;
        }

        if (typeof payload.total === 'number') {
          setTotal(payload.total);
        }

        if (payload.type === 'started') {
          setProgress({
            status: 'running',
            processed: payload.processed ?? 0,
            total: payload.total ?? job.total,
            message: payload.message ?? 'Evaluation started'
          });
          if (pageRef.current === 0) {
            setEvaluations([]);
          }
          setLiveNotice(false);
          setLoading(false);
          setCancelling(false);
          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
          }
          return;
        }

        if (payload.type === 'evaluation') {
          setLoading(false);
          setProgress((prev) => ({
            status: 'running',
            processed: payload.processed ?? prev.processed,
            total: payload.total ?? prev.total,
            message: payload.message ?? prev.message
          }));

          const payloadBatchId = payload.batch_id ?? job.batch_id ?? undefined;
          if (payloadBatchId && selectedBatchIdRef.current !== payloadBatchId) {
            setLiveNotice(true);
            return;
          }

          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
          }

          if (payload.evaluation) {
            const evaluation = mapEvaluation(payload.evaluation);
            setLiveEvaluations((prev) => {
              const next = [evaluation, ...prev];
              return next.slice(0, 20);
            });
            const currentFilters = filtersRef.current;
            if (pageRef.current === 0 && matchesFilters(evaluation, currentFilters)) {
              setEvaluations((prev) => {
                const next = applyEvaluationUpdate(prev, evaluation, currentFilters.sort);
                return next.slice(0, PAGE_SIZE);
              });
            } else if (pageRef.current !== 0) {
              setLiveNotice(true);
            }
          }
          return;
        }

        if (payload.type === 'progress') {
          setLoading(false);
          setProgress((prev) => ({
            status: payload.message === 'cancellation requested' ? 'cancelling' : prev.status,
            processed: payload.processed ?? prev.processed,
            total: payload.total ?? prev.total,
            message: payload.message ?? prev.message
          }));
          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
          }
          setEvaluationMessage('Evaluation running…');
          return;
        }

        if (payload.type === 'complete') {
          setProgress((prev) => ({
            status: 'complete',
            processed: payload.processed ?? prev.processed,
            total: payload.total ?? prev.total,
            message: payload.message ?? 'Evaluation complete'
          }));
          setJob(null);
          setLiveNotice(false);
          setCancelling(false);
          const refreshedBatchId = payload.batch_id ?? job.batch_id;
          if (refreshedBatchId) {
            loadBatches(refreshedBatchId).catch((err) => console.error(err));
          }
          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
          }
          setEvaluationMessage('Evaluation complete.');
          const reload = loadResultsRef.current;
          if (reload) {
            reload().catch((err) => console.error(err));
          }
          return;
        }

        if (payload.type === 'cancelled') {
          setProgress((prev) => ({
            status: 'cancelled',
            processed: payload.processed ?? prev.processed,
            total: payload.total ?? prev.total,
            message: payload.message ?? 'Evaluation cancelled'
          }));
          setJob(null);
          setCancelling(false);
          const refreshedBatchId = payload.batch_id ?? job.batch_id;
          if (refreshedBatchId) {
            loadBatches(refreshedBatchId).catch((err) => console.error(err));
          }
          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
          }
          setEvaluationMessage('Evaluation cancelled.');
          return;
        }

        if (payload.type === 'error') {
          setProgress((prev) => ({
            status: 'error',
            processed: payload.processed ?? prev.processed,
            total: payload.total ?? prev.total,
            message: payload.message ?? 'Evaluation failed'
          }));
          setJob(null);
          setCancelling(false);
          const refreshedBatchId = payload.batch_id ?? job.batch_id;
          if (refreshedBatchId) {
            loadBatches(refreshedBatchId).catch((err) => console.error(err));
          }
          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
          }
          setEvaluationMessage(payload.message ?? 'Evaluation failed.');
          return;
        }
    } catch (err) {
      console.error('Failed to parse evaluation event', err);
    }
  };

  return () => {
    socket.close();
    socketRef.current = null;
  };
}, [job, loadBatches]);

  useEffect(() => {
    if (page === 0) {
      setLiveNotice(false);
    }
  }, [page]);

  return (
    <>
      <main className="min-h-screen bg-slate-950 text-slate-100">
        <div className="mx-auto max-w-6xl px-4 py-10 space-y-8">
          <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h1 className="text-2xl font-semibold">Domain Risk Evaluation</h1>
              <p className="text-sm text-slate-400">Score trademark and vice risk in bulk with AI explanations.</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={handleBatchRefresh}
                disabled={batchLoading}
                className={clsx(
                  'rounded-full border border-slate-700 px-4 py-1.5 text-sm text-slate-200 transition-colors',
                  batchLoading ? 'cursor-not-allowed opacity-60' : 'hover:bg-slate-800'
                )}
              >
                Refresh data
              </button>
              <button
                type="button"
                onClick={() => setExplorerOpen(true)}
                className="rounded-full bg-slate-100 px-4 py-1.5 text-sm font-medium text-slate-900 hover:bg-white"
              >
                Browse CSVs
              </button>
            </div>
          </header>

          <div className="grid gap-6 lg:grid-cols-[320px_1fr]">
            <aside className="space-y-6">
              <UploadPane onProcess={handleProcess} onEvaluate={handleEvaluate} busy={busy || isEvaluating} />

              <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4">
                <h3 className="text-xs uppercase tracking-wide text-slate-500">This page</h3>
                <div className="mt-3 grid grid-cols-2 gap-3 text-sm text-slate-100">
                  {summaryItems.map((item) => (
                    <div key={item.label} className={clsx('rounded-xl px-3 py-2', item.accent)}>
                      <p className="text-xs uppercase tracking-wide text-slate-200/70">{item.label}</p>
                      <p className="text-lg font-semibold">{item.value.toLocaleString()}</p>
                    </div>
                  ))}
                </div>
              </section>

              {progress.status !== 'idle' && (
                <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-xs uppercase tracking-wide text-slate-500">Evaluation</p>
                      <p className="text-sm font-medium text-slate-100">
                        {progress.processed.toLocaleString()} / {progress.total.toLocaleString()} processed
                      </p>
                    </div>
                    <span className="text-xs text-slate-400">{progress.status}</span>
                  </div>
                  <div className="h-2 w-full rounded-full bg-slate-800">
                    <div
                      className="h-full rounded-full bg-slate-100 transition-all"
                      style={{ width: `${progressPercent}%` }}
                    />
                  </div>
                  {progressNote && <p className="text-xs text-slate-400">{progressNote}</p>}
                  {liveNotice && page > 0 && (
                    <button
                      type="button"
                      onClick={() => {
                        setPage(0);
                        setLiveNotice(false);
                        const reload = loadResultsRef.current;
                        if (reload) {
                          reload().catch((err) => console.error(err));
                        }
                      }}
                      className="text-xs font-medium text-slate-200 hover:text-white"
                    >
                      Jump to newest results
                    </button>
                  )}
                  <div className="flex flex-wrap gap-2 text-xs">
                    {(progress.status === 'running' || progress.status === 'cancelling') && (
                      <button
                        type="button"
                        onClick={handleCancel}
                        disabled={cancelling}
                        className={clsx(
                          'rounded-full border border-red-500/60 px-3 py-1 text-red-200',
                          cancelling ? 'cursor-not-allowed opacity-60' : 'hover:bg-red-500/10'
                        )}
                      >
                        {cancelling ? 'Cancelling…' : 'Stop evaluation'}
                      </button>
                    )}
                    {progress.status !== 'running' && progress.status !== 'cancelling' && progress.total > 0 && (
                      <button
                        type="button"
                        onClick={() => {
                          void handleResume();
                        }}
                        disabled={busy}
                        className={clsx(
                          'rounded-full border border-slate-700 px-3 py-1 text-slate-200',
                          busy ? 'cursor-not-allowed opacity-60' : 'hover:bg-slate-800'
                        )}
                      >
                        Resume evaluation
                      </button>
                    )}
                  </div>
                </section>
              )}

              <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 space-y-2">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs uppercase tracking-wide text-slate-500">Selected dataset</h3>
                  <button
                    type="button"
                    onClick={() => setExplorerOpen(true)}
                    className="text-xs text-slate-300 hover:text-white"
                  >
                    Manage
                  </button>
                </div>
                {selectedBatch ? (
                  <div className="space-y-2 text-sm text-slate-300">
                    <p className="font-medium text-slate-100 break-words">{selectedBatch.name}</p>
                    <p className="text-xs text-slate-500">
                      Owner: {selectedBatch.owner} • {new Date(selectedBatch.created_at).toLocaleDateString()}
                    </p>
                    <p className="text-xs text-slate-400">
                      {selectedBatch.processed_domains.toLocaleString()} processed • {remainingForSelected.toLocaleString()} remaining
                    </p>
                    <div className="flex flex-wrap gap-2 text-xs">
                      <button
                        type="button"
                        onClick={() => {
                          void handleEvaluate({ resume: true });
                        }}
                        className="rounded-full border border-slate-700 px-3 py-1 text-slate-200 hover:bg-slate-800"
                      >
                        Resume
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          void handleEvaluate({ force: true, batchId: selectedBatch.id });
                        }}
                        className="rounded-full border border-slate-700 px-3 py-1 text-slate-200 hover:bg-slate-800"
                      >
                        Force re-run
                      </button>
                    </div>
                  </div>
                ) : (
                  <p className="text-xs text-slate-400">No dataset selected. Upload a CSV or open the explorer.</p>
                )}
              </section>

              <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs uppercase tracking-wide text-slate-500">Live stream</h3>
                  {liveSlice.length > 0 && (
                    <button
                      type="button"
                      onClick={() => setLiveEvaluations([])}
                      className="text-xs text-slate-300 hover:text-white"
                    >
                      Clear
                    </button>
                  )}
                </div>
                {liveSlice.length === 0 ? (
                  <p className="text-xs text-slate-400">Start or resume a batch to watch AI decisions arrive in real time.</p>
                ) : (
                  <ul className="space-y-2 text-xs">
                    {liveSlice.map((item) => (
                      <li key={item.id} className="rounded-xl border border-slate-800 bg-slate-900/70 px-3 py-2">
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-slate-100 break-all">{item.domain}</span>
                          <span className="rounded-full border border-slate-700 px-2 py-0.5 text-[11px] uppercase text-slate-300">
                            {item.overall_recommendation}
                          </span>
                        </div>
                        <p className="mt-1 text-[11px] text-slate-400 break-words">{item.explanation || '—'}</p>
                      </li>
                    ))}
                  </ul>
                )}
              </section>

              <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <h3 className="text-xs uppercase tracking-wide text-slate-500">Recent batches</h3>
                  <button
                    type="button"
                    onClick={handleBatchRefresh}
                    disabled={batchLoading}
                    className={clsx(
                      'text-xs text-slate-300 hover:text-white',
                      batchLoading && 'cursor-not-allowed opacity-60'
                    )}
                  >
                    Refresh
                  </button>
                </div>
                {recentBatches.length === 0 ? (
                  <p className="text-xs text-slate-400">No uploads yet.</p>
                ) : (
                  <div className="space-y-2">
                    {recentBatches.map((batch) => {
                      const isSelected = batch.id === selectedBatch?.id;
                      const remaining = Math.max(batch.unique_domains - batch.processed_domains, 0);
                      return (
                        <button
                          key={batch.id}
                          type="button"
                          onClick={() => handleSelectBatch(batch)}
                          className={clsx(
                            'w-full rounded-xl border border-slate-800 bg-slate-900/60 px-3 py-2 text-left text-xs transition-colors hover:border-slate-600 hover:bg-slate-900',
                            isSelected && 'border-slate-200'
                          )}
                        >
                          <div className="flex items-center justify-between text-slate-200">
                            <span className="break-words">{batch.name}</span>
                            <span className="text-slate-500">{new Date(batch.created_at).toLocaleDateString()}</span>
                          </div>
                          <div className="mt-1 flex justify-between text-[11px] text-slate-400">
                            <span>
                              {batch.processed_domains.toLocaleString()} / {batch.unique_domains.toLocaleString()} done
                            </span>
                            <span>{remaining.toLocaleString()} left</span>
                          </div>
                        </button>
                      );
                    })}
                  </div>
                )}
              </section>
            </aside>

            <ResultsTable
              data={evaluations}
              total={total}
              loading={loading || busy || (isEvaluating && evaluations.length === 0)}
              page={page}
              pageSize={PAGE_SIZE}
              onPageChange={handlePageChange}
              onQueryChange={handleQueryChange}
              onExport={handleExport}
              tldOptions={tldOptions}
              filters={{
                q: filterQuery,
                minScore: filterMinScore,
                minViceScore: filterMinViceScore,
                tld: filterTld,
                recommendation: filterRecommendation,
                sort: filterSort,
              }}
              batchName={selectedBatch?.name}
            />
          </div>
        </div>
      </main>

      <CsvExplorer
        open={explorerOpen}
        batches={batches}
        selectedId={selectedBatch?.id ?? null}
        onSelect={handleSelectBatch}
        onClose={() => setExplorerOpen(false)}
      />
    </>
  );
}
