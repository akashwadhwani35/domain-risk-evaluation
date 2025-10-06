import axios from 'axios';
import type {
  EvaluateRequest,
  EvaluateResponse,
  ResultsQuery,
  UploadResponse,
  EvaluationDTO,
  StartEvaluationResponse,
  ConfigResponse,
  EvaluationStatusResponse,
  BatchesResponse,
  BatchDTO,
  BatchRequestDTO
} from '../types';

const API_BASE = import.meta.env.VITE_API_BASE ?? 'http://localhost:2000/api';

const client = axios.create({
  baseURL: API_BASE,
  timeout: 120000
});

export async function uploadFiles(form: FormData): Promise<UploadResponse> {
  const response = await client.post<UploadResponse>('/upload', form, {
    headers: { 'Content-Type': 'multipart/form-data' }
  });
  return response.data;
}

export async function triggerEvaluation(payload: EvaluateRequest): Promise<StartEvaluationResponse> {
  const response = await client.post<StartEvaluationResponse>('/evaluate', payload);
  return response.data;
}

export async function cancelEvaluation(jobId: string): Promise<void> {
  await client.delete(`/evaluate/${jobId}`);
}

export async function fetchConfig(): Promise<ConfigResponse> {
  const response = await client.get<ConfigResponse>('/config');
  return response.data;
}

export async function fetchResults(query: ResultsQuery): Promise<EvaluateResponse> {
  const response = await client.get<EvaluateResponse>('/results', {
    params: {
      q: query.q,
      minScore: query.minScore,
      minViceScore: query.minViceScore,
      tld: query.tld,
      recommendation: query.recommendation,
      sort: query.sort,
      page: query.page,
      pageSize: query.pageSize,
      batch_id: query.batchId
    }
  });
  return response.data;
}

export async function exportResults(format: 'csv' | 'json', options?: { batchId?: number }): Promise<Blob> {
  const endpoint = format === 'csv' ? '/export.csv' : '/export.json';
  const response = await client.get(endpoint, {
    responseType: 'blob',
    params: {
      batch_id: options?.batchId
    }
  });
  return response.data;
}

export async function listBatches(page = 0, pageSize = 25): Promise<BatchesResponse> {
  const response = await client.get<BatchesResponse>('/batches', {
    params: { page, pageSize }
  });
  return response.data;
}

export async function fetchBatch(batchId: number): Promise<BatchDTO> {
  const response = await client.get<BatchDTO>(`/batches/${batchId}`);
  return response.data;
}

export async function fetchRequestStatus(requestId: number): Promise<BatchRequestDTO> {
  const response = await client.get<BatchRequestDTO>(`/requests/${requestId}/status`);
  return response.data;
}

export function mapEvaluation(dto: EvaluationDTO) {
  return {
    ...dto,
    vice_categories: dto.vice_categories ?? [],
    explanation: dto.explanation ?? '',
    commercial_override: Boolean(dto.commercial_override),
    commercial_source: dto.commercial_source ?? '',
    commercial_similarity: typeof dto.commercial_similarity === 'number' ? dto.commercial_similarity : 0
  };
}

export function buildWebSocketURL(path: string) {
  const base = client.defaults.baseURL ?? API_BASE;
  const normalizedBase = base.endsWith('/') ? base : `${base}/`;
  const target = new URL(path, normalizedBase);
  target.protocol = target.protocol === 'https:' ? 'wss:' : 'ws:';
  return target.toString();
}

export async function fetchEvaluationStatus(): Promise<EvaluationStatusResponse> {
  const response = await client.get<EvaluationStatusResponse>('/evaluate/status');
  return response.data;
}
