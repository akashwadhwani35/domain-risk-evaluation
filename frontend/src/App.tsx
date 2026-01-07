import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import clsx from 'clsx';
import { isAxiosError } from 'axios';
import UploadPane from './components/UploadPane';
import ResultsTable from './components/ResultsTable';
import CsvExplorer from './components/CsvExplorer';
import Dashboard from './components/Dashboard';
import OverrideModal from './components/OverrideModal';
import AILearningDashboard from './components/AILearningDashboard';
import EvaluationsQueue from './components/EvaluationsQueue';
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

type ViewMode = 'dashboard' | 'evaluations' | 'results' | 'ai_learning';

type SidebarState = 'expanded' | 'collapsed';

const normalizeSort = (sort?: string) => (sort && sort.trim() !== '' ? sort : DEFAULT_SORT);

const matchesFilters = (row: EvaluationDTO, filters: FiltersState) => {
  if (filters.tld) {
    const tld = filters.tld.startsWith('.') ? filters.tld.toLowerCase() : `.${filters.tld.toLowerCase()}`;
    if (!row.domain.toLowerCase().endsWith(tld)) {
      return false;
    }
  }
  if (filters.recommendation) {
    // Match if either trademark or vice recommendation matches the filter
    const filterRec = normalizeRecommendation(filters.recommendation);
    const tmRec = normalizeRecommendation(row.trademark_recommendation);
    const viceRec = normalizeRecommendation(row.vice_recommendation);
    if (tmRec !== filterRec && viceRec !== filterRec) {
      return false;
    }
  }
  if (filters.q) {
    const query = filters.q.toLowerCase();
    const domainMatch = row.domain.toLowerCase().includes(query);
    if (!domainMatch) {
      return false;
    }
  }
  return true;
};

// Helper to normalize legacy recommendations to new 3-tier format
const normalizeRecommendation = (rec: string): string => {
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
  const [viewMode, setViewMode] = useState<ViewMode>('dashboard');
  const [sidebarState, setSidebarState] = useState<SidebarState>('expanded');
  const [selectedEvaluation, setSelectedEvaluation] = useState<EvaluationDTO | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const filtersRef = useRef(filters);
  const pageRef = useRef(page);
  const loadResultsRef = useRef<() => Promise<void>>();
  const selectedBatchIdRef = useRef<number | null>(null);

  const isEvaluating = Boolean(
    job && (progress.status === 'running' || progress.status === 'cancelling')
  );

  const filterQuery = filters.q;
  const filterTld = filters.tld;
  const filterRecommendation = filters.recommendation;
  const filterSort = normalizeSort(filters.sort);

  const loadResults = useCallback(async () => {
    setLoading(true);
    try {
      const batchId = selectedBatch?.id;
      const response = await fetchResults({
        q: filterQuery,
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
  }, [filterQuery, filterTld, filterRecommendation, filterSort, page, selectedBatch?.id]);

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

  const handleRowClick = useCallback((evaluation: EvaluationDTO) => {
    setSelectedEvaluation(evaluation);
  }, []);

  const handleOverrideCreated = useCallback(() => {
    setSelectedEvaluation(null);
    // Refresh results to show the updated evaluation
    const reload = loadResultsRef.current;
    if (reload) {
      reload().catch((err) => console.error(err));
    }
  }, []);

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
    const tmCounts = { YES_RISK: 0, POTENTIAL_RISK: 0, NO_RISK: 0 };
    const viceCounts = { YES_RISK: 0, POTENTIAL_RISK: 0, NO_RISK: 0 };
    evaluations.forEach((row) => {
      const tmKey = normalizeRecommendation(row.trademark_recommendation ?? '');
      if (tmKey in tmCounts) {
        tmCounts[tmKey as keyof typeof tmCounts] += 1;
      }
      const viceKey = normalizeRecommendation(row.vice_recommendation ?? '');
      if (viceKey in viceCounts) {
        viceCounts[viceKey as keyof typeof viceCounts] += 1;
      }
    });
    return { trademark: tmCounts, vice: viceCounts };
  }, [evaluations]);

  const summaryItems = useMemo(
    () => [
      { label: 'TM Risk', value: summaryCounts.trademark.YES_RISK + summaryCounts.trademark.POTENTIAL_RISK, accent: 'bg-red-100 text-red-800' },
      { label: 'Vice Risk', value: summaryCounts.vice.YES_RISK + summaryCounts.vice.POTENTIAL_RISK, accent: 'bg-amber-100 text-amber-800' },
      { label: 'Total', value: evaluations.length, accent: 'bg-blue-100 text-blue-800' },
    ],
    [summaryCounts, evaluations.length]
  );

  const progressPercent = useMemo(() => {
    if (!progress.total) return 0;
    return Math.min(100, Math.round((progress.processed / progress.total) * 100));
  }, [progress.processed, progress.total]);

  const liveSlice = useMemo(() => liveEvaluations.slice(0, 6), [liveEvaluations]);
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
      <main className="min-h-screen text-[var(--text)]">
        <div className="mx-auto w-full max-w-[1440px] px-6 py-8 space-y-6">
          <header className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div className="flex items-center gap-4">
              <button
                type="button"
                onClick={() => setSidebarState(sidebarState === 'expanded' ? 'collapsed' : 'expanded')}
                className="rounded-lg border border-[var(--line)] p-2 text-[var(--muted)] hover:bg-[var(--surface-2)] hover:text-[var(--text)] transition-colors lg:hidden"
                aria-label="Toggle sidebar"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                </svg>
              </button>
              <div>
                <h1 className="text-2xl font-semibold">Domain Risk Evaluation</h1>
                <p className="text-[var(--muted)]">Trademark and vice risk assessment</p>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {/* View Toggle */}
              <div className="flex items-center rounded-lg border border-[var(--line)] p-1 bg-[var(--surface)]">
                <button
                  type="button"
                  onClick={() => setViewMode('dashboard')}
                  className={clsx(
                    'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    viewMode === 'dashboard'
                      ? 'bg-[var(--text)] text-white'
                      : 'text-[var(--muted)] hover:text-[var(--text)]'
                  )}
                >
                  Dashboard
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode('evaluations')}
                  className={clsx(
                    'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    viewMode === 'evaluations'
                      ? 'bg-[var(--text)] text-white'
                      : 'text-[var(--muted)] hover:text-[var(--text)]'
                  )}
                >
                  Evaluations
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode('results')}
                  className={clsx(
                    'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    viewMode === 'results'
                      ? 'bg-[var(--text)] text-white'
                      : 'text-[var(--muted)] hover:text-[var(--text)]'
                  )}
                >
                  Results
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode('ai_learning')}
                  className={clsx(
                    'rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    viewMode === 'ai_learning'
                      ? 'bg-[var(--text)] text-white'
                      : 'text-[var(--muted)] hover:text-[var(--text)]'
                  )}
                >
                  AI Learning
                </button>
              </div>

              <button
                type="button"
                onClick={handleBatchRefresh}
                disabled={batchLoading}
                className={clsx(
                  'rounded-lg border border-[var(--line)] px-4 py-2 text-sm font-medium',
                  batchLoading ? 'cursor-not-allowed opacity-60 text-[var(--muted)]' : 'text-[var(--text)] hover:bg-[var(--surface-2)]'
                )}
              >
                Refresh
              </button>
              <button
                type="button"
                onClick={() => setExplorerOpen(true)}
                className="rounded-lg bg-[var(--text)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 transition-opacity"
              >
                Browse Datasets
              </button>
            </div>
          </header>

          <div className={clsx(
              'grid gap-6 transition-all duration-300',
              sidebarState === 'expanded' ? 'lg:grid-cols-[300px_minmax(0,1fr)]' : 'lg:grid-cols-1'
            )}>
            <aside className={clsx(
              'space-y-4 transition-all duration-300',
              sidebarState === 'collapsed' && 'hidden lg:hidden'
            )}>
              <UploadPane onProcess={handleProcess} onEvaluate={handleEvaluate} busy={busy || isEvaluating} />

              <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
                <div className="flex items-center justify-between">
                  <h3 className="font-semibold text-[var(--text)]">Active Batch</h3>
                  <button
                    type="button"
                    onClick={() => setExplorerOpen(true)}
                    className="text-sm text-[var(--muted)] hover:text-[var(--text)]"
                  >
                    Switch
                  </button>
                </div>
                {selectedBatch ? (
                  <div className="mt-3 space-y-3 text-[var(--text)]">
                    <div>
                      <p className="font-semibold break-words">{selectedBatch.name}</p>
                      <p className="text-sm text-[var(--muted)]">
                        {selectedBatch.owner} · {new Date(selectedBatch.created_at).toLocaleDateString()}
                      </p>
                    </div>
                    <div className="flex items-center justify-between text-sm text-[var(--muted)]">
                      <span>{selectedBatch.processed_domains.toLocaleString()} processed</span>
                      <span>{remainingForSelected.toLocaleString()} remaining</span>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button
                        type="button"
                        onClick={() => {
                          void handleEvaluate({ resume: true });
                        }}
                        className="rounded-lg bg-[var(--text)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
                      >
                        Resume
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          void handleEvaluate({ force: true, batchId: selectedBatch.id });
                        }}
                        className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm font-medium text-[var(--text)] hover:bg-[var(--surface-2)]"
                      >
                        Force Re-run
                      </button>
                    </div>
                  </div>
                ) : (
                  <p className="mt-3 text-sm text-[var(--muted)]">No dataset selected. Upload or browse a batch.</p>
                )}
              </section>

              <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5 space-y-3">
                <div className="flex items-center justify-between">
                  <h3 className="font-semibold text-[var(--text)]">Evaluation Status</h3>
                  <span className={clsx(
                    'text-sm font-medium px-2 py-0.5 rounded',
                    progress.status === 'running' && 'bg-blue-100 text-blue-700',
                    progress.status === 'complete' && 'bg-green-100 text-green-700',
                    progress.status === 'error' && 'bg-red-100 text-red-700',
                    progress.status === 'idle' && 'bg-gray-100 text-gray-600'
                  )}>{progress.status}</span>
                </div>
                <div className="text-[var(--text)]">
                  {progress.processed.toLocaleString()} / {progress.total.toLocaleString()} processed
                </div>
                <div className="h-2 w-full rounded-full bg-[var(--surface-2)]">
                  <div
                    className="h-full rounded-full bg-[var(--accent)] transition-all"
                    style={{ width: `${progressPercent}%` }}
                  />
                </div>
                {progressNote && <p className="text-sm text-[var(--muted)]">{progressNote}</p>}
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
                    className="text-sm font-medium text-[var(--accent)]"
                  >
                    Jump to newest results
                  </button>
                )}
                <div className="flex flex-wrap gap-2">
                  {(progress.status === 'running' || progress.status === 'cancelling') && (
                    <button
                      type="button"
                      onClick={handleCancel}
                      disabled={cancelling}
                      className={clsx(
                        'rounded-lg border px-3 py-2 text-sm font-medium',
                        cancelling
                          ? 'cursor-not-allowed border-gray-200 text-gray-400'
                          : 'border-red-200 text-red-600 hover:bg-red-50'
                      )}
                    >
                      {cancelling ? 'Cancelling…' : 'Stop'}
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
                        'rounded-lg border px-3 py-2 text-sm font-medium',
                        busy
                          ? 'cursor-not-allowed border-[var(--line)] text-[var(--muted)]'
                          : 'border-[var(--line)] text-[var(--text)] hover:bg-[var(--surface-2)]'
                      )}
                    >
                      Resume
                    </button>
                  )}
                </div>
              </section>

              <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5">
                <h3 className="font-semibold text-[var(--text)]">Page Summary</h3>
                <div className="mt-3 grid grid-cols-3 gap-2">
                  {summaryItems.map((item) => (
                    <div key={item.label} className={clsx('rounded-lg px-3 py-2 text-center', item.accent)}>
                      <p className="text-xs font-medium">{item.label}</p>
                      <p className="text-lg font-bold">{item.value.toLocaleString()}</p>
                    </div>
                  ))}
                </div>
              </section>
            </aside>

            <div className="space-y-6">
              {viewMode === 'dashboard' ? (
                <Dashboard
                  selectedBatch={selectedBatch}
                  onSelectBatch={handleSelectBatch}
                  onSwitchToResults={() => setViewMode('results')}
                />
              ) : viewMode === 'evaluations' ? (
                <EvaluationsQueue
                  batchId={selectedBatch?.id}
                  onOverrideCreated={() => {
                    const reload = loadResultsRef.current;
                    if (reload) {
                      reload().catch((err) => console.error(err));
                    }
                  }}
                />
              ) : viewMode === 'ai_learning' ? (
                <AILearningDashboard batchId={selectedBatch?.id} />
              ) : (
                <>
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
                      tld: filterTld,
                      recommendation: filterRecommendation,
                      sort: filterSort,
                    }}
                    batchName={selectedBatch?.name}
                    onRowClick={handleRowClick}
                  />

                  {liveSlice.length > 0 && (
                    <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-5 space-y-3">
                      <div className="flex items-center justify-between">
                        <h3 className="font-semibold text-[var(--text)]">Live Decisions</h3>
                        <button
                          type="button"
                          onClick={() => setLiveEvaluations([])}
                          className="text-sm text-[var(--muted)] hover:text-[var(--text)]"
                        >
                          Clear
                        </button>
                      </div>
                      <ul className="space-y-2">
                        {liveSlice.map((item) => (
                          <li key={item.id} className="rounded-lg border border-[var(--line)] bg-[var(--surface-2)] px-4 py-3">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-medium text-[var(--text)] break-all">{item.domain}</span>
                              <div className="flex gap-1">
                                <span className="rounded-md border border-[var(--line)] px-2 py-1 text-xs text-[var(--muted)]">
                                  TM: {item.trademark_recommendation}
                                </span>
                                <span className="rounded-md border border-[var(--line)] px-2 py-1 text-xs text-[var(--muted)]">
                                  Vice: {item.vice_recommendation}
                                </span>
                              </div>
                            </div>
                            <p className="mt-2 text-sm text-[var(--muted)] break-words line-clamp-2">{item.explanation || '—'}</p>
                          </li>
                        ))}
                      </ul>
                    </section>
                  )}
                </>
              )}
            </div>
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

      {selectedEvaluation && (
        <OverrideModal
          evaluation={selectedEvaluation}
          onClose={() => setSelectedEvaluation(null)}
          onOverrideCreated={handleOverrideCreated}
        />
      )}
    </>
  );
}
