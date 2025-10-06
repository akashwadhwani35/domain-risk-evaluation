import { useCallback, useEffect, useRef, useState } from 'react';
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
import type { BatchDTO, EvaluationDTO, EvaluationEvent, StartEvaluationResponse } from './types';

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
    console.log('[loadResults] Starting fetch with params:', {
      filterQuery,
      filterMinScore,
      filterMinViceScore,
      filterTld,
      filterRecommendation,
      filterSort,
      page,
      pageSize: PAGE_SIZE,
      batchId: selectedBatch?.id
    });
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
      console.log('[loadResults] API response:', {
        itemsCount: response.items?.length,
        total: response.total,
        firstItem: response.items?.[0],
        rawResponse: response
      });
      const mapped = response.items.map(mapEvaluation);
      console.log('[loadResults] Mapped evaluations:', {
        count: mapped.length,
        firstMapped: mapped[0]
      });
      setEvaluations(mapped);
      setTotal(response.total);
      console.log('[loadResults] State updated - evaluations count:', mapped.length, 'total:', response.total);
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
      console.log('[config] loaded', config);
    } catch (err) {
      console.error('Failed to load config', err);
    }
  }, []);

  const loadBatches = useCallback(async (preferredId?: number) => {
    console.log('[loadBatches] Starting - preferredId:', preferredId, 'selectedBatchIdRef.current:', selectedBatchIdRef.current);
    setBatchLoading(true);
    try {
      const response = await listBatches();
      const next = Array.isArray(response.items) ? response.items : [];
      console.log('[loadBatches] Fetched batches:', next.length, 'batches');
      setBatches(next);

      const fallbackId = preferredId ?? selectedBatchIdRef.current ?? (next.length > 0 ? next[0].id : null);
      console.log('[loadBatches] Fallback batch ID to select:', fallbackId);
      if (fallbackId) {
        const match = next.find((batch) => batch.id === fallbackId);
        if (match) {
          console.log('[loadBatches] Found matching batch in list:', match.name);
          setSelectedBatch(match);
        } else {
          console.log('[loadBatches] Batch not in list, fetching details for ID:', fallbackId);
          const details = await fetchBatch(fallbackId);
          console.log('[loadBatches] Fetched batch details:', details.name);
          setSelectedBatch(details);
          setBatches((prev) => {
            const filtered = prev.filter((item) => item.id !== details.id);
            return [details, ...filtered];
          });
        }
      } else {
        console.log('[loadBatches] No batch to select');
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
      console.log('[ws] opened', { jobId: job.job_id, total: job.total });
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
      console.log('[ws] closed', { jobId: job.job_id });
      socketRef.current = null;
    };

    socket.onmessage = (event) => {
      try {
        const payload: EvaluationEvent = JSON.parse(event.data);
        console.log('[ws] message', payload);
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
            setEvaluationMessage(payload.message ?? 'Evaluation failed.');
        }
          if (payload.batch_id && selectedBatchIdRef.current === payload.batch_id) {
            touchSelectedBatch((current) => ({
              ...current,
              processed_domains: payload.processed ?? current.processed_domains,
              last_evaluated_at: new Date().toISOString()
            }));
            setEvaluationMessage(payload.message ?? 'Evaluation failed.');
        }
          setEvaluationMessage(payload.message ?? 'Evaluation failed.');
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

  const handleProcess = async (formData: FormData) => {
    setBusy(true);
    try {
      const response = await uploadFiles(formData);
      setLiveEvaluations([]);
      setEvaluationMessage(null);
      await loadBatches(response.batch_id);
      await refreshConfig();
      return response;
    } finally {
      setBusy(false);
    }
  };

  const evaluateBatch = useCallback(
    async ({ resume = false, force = false, batchId }: { resume?: boolean; force?: boolean; batchId?: number } = {}) => {
      const targetBatchId = batchId ?? selectedBatchIdRef.current;
      if (!targetBatchId) {
        setProgress({ status: 'error', processed: 0, total: 0, message: 'Select a batch before running an evaluation.' });
        return;
      }

      setBusy(true);
      setLiveEvaluations([]);
      try {
        const response = await triggerEvaluation({
          batch_id: targetBatchId,
          limit: 5000,
          offset: 0,
          resume,
          force
        });
        setJob(response);
        setPage(0);
        setEvaluations([]);
        setTotal(response.total);
        setLoading(false);
        setCancelling(false);
        setProgress({ status: 'running', processed: 0, total: response.total, message: 'Evaluation started' });
        setEvaluationMessage('Evaluation running…');
        await loadBatches(response.batch_id);
      } catch (err) {
        if (isAxiosError(err) && err.response?.status === 409) {
          setEvaluationMessage('An evaluation is already running for this CSV. Wait for it to finish or cancel it first.');
          setProgress((prev) => ({
            status: 'error',
            processed: prev.processed,
            total: prev.total,
            message: 'Another evaluation is already running. Wait for it to finish or cancel it first.'
          }));
          syncEvaluationStatus().catch((statusErr) => console.error('Failed to refresh evaluation status', statusErr));
        } else {
          console.error('Failed to start evaluation', err);
          setEvaluationMessage('Failed to start evaluation. Check logs and try again.');
          setProgress((prev) => ({
            status: 'error',
            processed: prev.processed,
            total: prev.total,
            message: 'Failed to start evaluation'
          }));
        }
      } finally {
        setBusy(false);
      }
    },
    [loadBatches]
  );

  const handleEvaluate = useCallback((resume?: boolean) => evaluateBatch({ resume: Boolean(resume) }), [evaluateBatch]);

  const handleResume = useCallback(() => evaluateBatch({ resume: true }), [evaluateBatch]);

  const handleCancel = async () => {
    if (!job) {
      return;
    }
    try {
      await cancelEvaluation(job.job_id);
      setProgress((prev) => ({
        status: 'cancelling',
        processed: prev.processed,
        total: prev.total,
        message: 'Cancellation requested…'
      }));
      setCancelling(true);
    } catch (err) {
      console.error('Failed to cancel evaluation', err);
      setCancelling(false);
      setProgress((prev) => ({
        status: 'error',
        processed: prev.processed,
        total: prev.total,
        message: 'Failed to cancel evaluation'
      }));
    }
  };

  const handlePageChange = (nextPage: number) => {
    setPage((current) => {
      const normalized = Math.max(0, nextPage);
      return current === normalized ? current : normalized;
    });
  };

  const handleQueryChange = useCallback((next: FiltersState) => {
    const normalized: FiltersState = {
      q: next.q?.trim() ? next.q.trim() : undefined,
      minScore: typeof next.minScore === 'number' ? next.minScore : undefined,
      minViceScore: typeof next.minViceScore === 'number' ? next.minViceScore : undefined,
      tld: next.tld?.trim() ? next.tld.trim() : undefined,
      recommendation: next.recommendation?.trim() ? next.recommendation.trim() : undefined,
      sort: normalizeSort(next.sort)
    };

    setFilters((current) => {
      if (
        current.q === normalized.q &&
        current.minScore === normalized.minScore &&
        current.minViceScore === normalized.minViceScore &&
        current.tld === normalized.tld &&
        current.recommendation === normalized.recommendation &&
        normalizeSort(current.sort) === normalizeSort(normalized.sort)
      ) {
        return current;
      }
      return normalized;
    });

    setPage(0);
  }, []);

  const handleExport = async (format: 'csv' | 'json') => {
    try {
      const batchId = selectedBatchIdRef.current ?? undefined;
      const blob = await exportResults(format, { batchId });
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      const suffix = batchId ? `-batch-${batchId}` : '';
      link.download = format === 'csv' ? `domain-risk-results${suffix}.csv` : `domain-risk-results${suffix}.json`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (err) {
      console.error('Export failed', err);
    }
  };

  const recentBatches = batches.slice(0, 6);

  const initialsForName = (value?: string) => {
    if (!value) return 'CSV';
    const parts = value.split(/\s+/).filter(Boolean).slice(0, 2);
    if (parts.length === 0) return 'CSV';
    return parts.map((part) => part.charAt(0).toUpperCase()).join('');
  };

  return (
    <>
      <main className="mx-auto w-full max-w-[1400px] px-4 lg:px-6 py-10 space-y-8">
        <header className="space-y-1">
          <h1 className="text-2xl font-semibold">Domain Risk Evaluation</h1>
          <p className="text-sm text-slate-400">
            Upload your domain CSVs once, reuse evaluations anytime, and keep trademark coverage up to date.
          </p>
        </header>

        <UploadPane onProcess={handleProcess} onEvaluate={handleEvaluate} busy={busy || isEvaluating} />

        {progress.status !== 'idle' && (
          <section className="bg-slate-900 border border-slate-800 rounded-xl p-4 shadow-lg text-sm">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <span className="text-slate-200 font-medium">
                {progress.status === 'running'
                  ? `Streaming results: ${progress.processed.toLocaleString()} / ${progress.total.toLocaleString()} processed`
                  : progress.status === 'cancelling'
                    ? progress.message || 'Cancellation requested…'
                    : progress.message || ''}
              </span>
              <div className="flex items-center gap-3">
                <span className="text-xs uppercase tracking-wide text-slate-500">
                  {progress.status === 'running'
                    ? 'Live'
                    : progress.status === 'cancelling'
                      ? 'Cancelling'
                      : progress.status}
                </span>
                {(progress.status === 'running' || progress.status === 'cancelling') && (
                  <button
                    type="button"
                    onClick={handleCancel}
                    disabled={cancelling}
                    className="text-xs font-medium text-red-400 hover:text-red-300 disabled:opacity-60 disabled:cursor-not-allowed"
                  >
                    {cancelling ? 'Cancelling…' : 'Stop evaluation'}
                  </button>
                )}
                {progress.status !== 'running' && total > 0 && (
                  <button
                    type="button"
                    onClick={handleResume}
                    disabled={busy}
                    className="text-xs font-medium text-brand-400 hover:text-brand-300 disabled:opacity-60 disabled:cursor-not-allowed"
                  >
                    Resume evaluation
                  </button>
                )}
              </div>
            </div>
            {progress.status !== 'running' && progress.message && (
              <p className="text-xs text-slate-400 mt-2">{progress.message}</p>
            )}
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
                className="mt-3 text-xs font-medium text-brand-500 hover:text-brand-600"
              >
                Jump to newest results
              </button>
            )}
          </section>
        )}

        <section className="bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-lg space-y-4">
          <header className="flex items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold text-slate-100">Live results (WebSocket)</h2>
              <p className="text-sm text-slate-400">Latest AI decisions stream in while the batch runs.</p>
            </div>
            <button
              type="button"
              onClick={() => setLiveEvaluations([])}
              className="px-3 py-1.5 text-xs rounded-lg border border-slate-700 text-slate-200 hover:bg-slate-800"
            >
              Clear
            </button>
          </header>

          {liveEvaluations.length === 0 ? (
            <p className="text-sm text-slate-400">No live events yet — start or resume an evaluation to watch updates here.</p>
          ) : (
            <ul className="max-h-72 overflow-y-auto pr-1 space-y-3 text-sm">
              {liveEvaluations.map((item) => (
                <li
                  key={item.id}
                  className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 flex flex-col gap-2"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="font-semibold text-slate-100">{item.domain}</span>
                    <span className="inline-flex items-center justify-center px-2 py-1 text-xs font-semibold rounded-full capitalize border border-slate-700 text-slate-200">
                      {item.overall_recommendation}
                    </span>
                  </div>
                  <p className="text-xs text-slate-300 whitespace-pre-line line-clamp-3">
                    {item.explanation || '—'}
                  </p>
                </li>
              ))}
            </ul>
          )}
        </section>

        {selectedBatch ? (
          <section className="bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-lg space-y-5">
            <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 className="text-lg font-semibold text-slate-100">Selected CSV: {selectedBatch.name}</h2>
                <p className="text-xs text-slate-400">
                  Owner: {selectedBatch.owner} • Uploaded {new Date(selectedBatch.created_at).toLocaleString()}
                </p>
                {selectedBatch.last_evaluated_at && (
                  <p className="text-xs text-slate-500">
                    Last evaluated {new Date(selectedBatch.last_evaluated_at).toLocaleString()}
                  </p>
                )}
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={handleBatchRefresh}
                  disabled={batchLoading}
                  className={clsx(
                    'px-3 py-2 rounded-lg border border-slate-700 text-slate-200 hover:bg-slate-800',
                    batchLoading && 'opacity-60 cursor-not-allowed'
                  )}
                >
                  Refresh stats
                </button>
                <button
                  type="button"
                  onClick={() => setExplorerOpen(true)}
                  className="px-3 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-100"
                >
                  View all CSVs
                </button>
              </div>
            </div>

            <dl className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4 text-sm">
              <div>
                <dt className="text-xs text-slate-500 uppercase tracking-wide">Rows</dt>
                <dd className="mt-1 text-base font-semibold text-slate-100">{selectedBatch.row_count.toLocaleString()}</dd>
              </div>
              <div>
                <dt className="text-xs text-slate-500 uppercase tracking-wide">Unique Domains</dt>
                <dd className="mt-1 text-base font-semibold text-slate-100">{selectedBatch.unique_domains.toLocaleString()}</dd>
              </div>
              <div>
                <dt className="text-xs text-slate-500 uppercase tracking-wide">Duplicates</dt>
                <dd className="mt-1 text-base font-semibold text-slate-100">{selectedBatch.duplicate_rows.toLocaleString()}</dd>
              </div>
              <div>
                <dt className="text-xs text-slate-500 uppercase tracking-wide">Evaluated</dt>
                <dd className="mt-1 text-base font-semibold text-slate-100">{selectedBatch.processed_domains.toLocaleString()}</dd>
              </div>
              <div>
                <dt className="text-xs text-slate-500 uppercase tracking-wide">Remaining</dt>
                <dd className="mt-1 text-base font-semibold text-slate-100">
                  {Math.max(selectedBatch.unique_domains - selectedBatch.processed_domains, 0).toLocaleString()}
                </dd>
              </div>
            </dl>

            <div className="flex flex-wrap gap-3 text-sm">
              <button
                type="button"
                onClick={() => evaluateBatch({ batchId: selectedBatch.id })}
                disabled={busy || isEvaluating}
                className="px-4 py-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-medium disabled:opacity-60 disabled:cursor-not-allowed"
              >
                Evaluate batch
              </button>
              <button
                type="button"
                onClick={() => evaluateBatch({ batchId: selectedBatch.id, resume: true })}
                disabled={busy || isEvaluating || selectedBatch.processed_domains >= selectedBatch.unique_domains}
                className="px-4 py-2 rounded-lg border border-slate-700 text-slate-200 hover:bg-slate-800 disabled:opacity-60 disabled:cursor-not-allowed"
              >
                Resume remaining
              </button>
              <button
                type="button"
                onClick={() => evaluateBatch({ batchId: selectedBatch.id, force: true })}
                disabled={busy || isEvaluating}
                className="px-4 py-2 rounded-lg border border-slate-700 text-slate-200 hover:bg-slate-800 disabled:opacity-60 disabled:cursor-not-allowed"
              >
                Force re-run
              </button>
            </div>
          </section>
        ) : (
          <section className="bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-lg">
            <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
              <div>
                <h2 className="text-lg font-semibold text-slate-100">No CSV selected</h2>
                <p className="text-sm text-slate-400">Upload a new CSV or open the explorer to pick an existing dataset.</p>
              </div>
              <button
                type="button"
                onClick={() => setExplorerOpen(true)}
                className="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-100"
              >
                Open CSV Explorer
              </button>
            </div>
          </section>
        )}

        <section className="bg-slate-900 border border-slate-800 rounded-xl p-6 shadow-lg space-y-4">
          <header className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 className="text-lg font-semibold text-slate-100">Recent CSV batches</h2>
              <p className="text-sm text-slate-400">Jump back into a dataset or browse everything from the explorer.</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                onClick={handleBatchRefresh}
                disabled={batchLoading}
                className={clsx(
                  'px-3 py-2 rounded-lg border border-slate-700 text-slate-200 hover:bg-slate-800',
                  batchLoading && 'opacity-60 cursor-not-allowed'
                )}
              >
                Refresh
              </button>
              <button
                type="button"
                onClick={() => setExplorerOpen(true)}
                className="px-3 py-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-medium"
              >
                Open Explorer
              </button>
            </div>
          </header>

          {recentBatches.length === 0 ? (
            <p className="text-sm text-slate-400">No CSV batches yet. Upload a CSV to get started.</p>
          ) : (
            <div className="grid gap-4 grid-cols-2 sm:grid-cols-3 lg:grid-cols-4">
              {recentBatches.map((batch) => {
                const isSelected = batch.id === selectedBatch?.id;
                const remaining = Math.max(batch.unique_domains - batch.processed_domains, 0);
                return (
                  <button
                    key={batch.id}
                    type="button"
                    onClick={() => handleSelectBatch(batch)}
                    className={clsx(
                      'rounded-2xl border border-slate-800 bg-slate-900/70 p-4 text-left transition-transform hover:-translate-y-1 hover:border-brand-500',
                      isSelected && 'ring-2 ring-brand-500 border-brand-500'
                    )}
                  >
                    <div className="flex items-center justify-between">
                      <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-brand-500 to-brand-400/80 flex items-center justify-center text-white font-semibold">
                        {initialsForName(batch.name)}
                      </div>
                      <span className="text-xs text-slate-500">{new Date(batch.created_at).toLocaleDateString()}</span>
                    </div>
                    <h3 className="mt-3 text-sm font-semibold text-slate-100 line-clamp-2">{batch.name}</h3>
                    <p className="text-xs text-slate-400 mt-1">Owner: {batch.owner}</p>
                    <div className="mt-3 text-xs text-slate-400 space-y-1">
                      <p>Total: {batch.row_count.toLocaleString()}</p>
                      <p>Evaluated: {batch.processed_domains.toLocaleString()}</p>
                      <p>Remaining: {remaining.toLocaleString()}</p>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </section>

        <ResultsTable
          data={evaluations}
          total={total}
          loading={
            loading || busy || (isEvaluating && evaluations.length === 0)
          }
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
            sort: filterSort
          }}
          batchName={selectedBatch?.name}
        />
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
