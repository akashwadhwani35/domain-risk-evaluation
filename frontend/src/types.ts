export interface UploadResponse {
  batch_id: number;
  batch_name: string;
  owner: string;
  row_count: number;
  unique_domains: number;
  existing_domains: number;
  duplicate_rows: number;
  processed_domains: number;
  marks_count: number;
}

export interface EvaluationDTO {
  id: number;
  domain: string;
  trademark_score: number;
  trademark_type: string;
  matched_trademark: string;
  trademark_confidence: number;
  vice_score: number;
  vice_categories: string[];
  vice_confidence: number;
  overall_recommendation: string;
  confidence: number;
  created_at: string;
  explanation: string;
  commercial_override: boolean;
  commercial_source: string;
  commercial_similarity: number;
}

export interface EvaluateResponse {
  items: EvaluationDTO[];
  total: number;
}

export interface StartEvaluationResponse {
  job_id: string;
  batch_id: number;
  request_id: number;
  total: number;
  started_at: string;
}

export interface EvaluationEvent {
  type: 'started' | 'evaluation' | 'progress' | 'complete' | 'error' | 'cancelled';
  job_id: string;
  batch_id?: number;
  total?: number;
  processed?: number;
  evaluation?: EvaluationDTO;
  batch?: EvaluationDTO[];
  message?: string;
  timestamp: string;
  reused?: boolean;
}

export interface ConfigResponse {
  seed_path: string;
  vice_terms_path: string;
  tlds?: string[];
  commercial_sales_records?: number;
}

export interface ResultsQuery {
  q?: string;
  minScore?: number;
  minViceScore?: number;
  tld?: string;
  recommendation?: string;
  sort?: string;
  page?: number;
  pageSize?: number;
  batchId?: number;
}

export interface EvaluateRequest {
  batch_id: number;
  limit?: number;
  offset?: number;
  resume?: boolean;
  force?: boolean;
}

export interface BatchDTO {
  id: number;
  name: string;
  owner: string;
  original_filename: string;
  row_count: number;
  unique_domains: number;
  existing_domains: number;
  duplicate_rows: number;
  processed_domains: number;
  created_at: string;
  last_evaluated_at?: string | null;
}

export interface BatchesResponse {
  items: BatchDTO[];
  total: number;
}

export interface BatchRequestDTO {
  id: number;
  batch_id: number;
  type: string;
  status: string;
  job_id: string;
  started_at: string;
  finished_at?: string | null;
}

export interface EvaluationStatusResponse {
  running: boolean;
  job_id?: string;
  batch_id?: number;
  request_id?: number;
  state?: string;
  message?: string;
  processed?: number;
  total?: number;
  last_evaluation?: EvaluationDTO;
}
