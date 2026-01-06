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
  manual_override: boolean;
  override_count: number;
  last_override_at?: string | null;
  feedback_used: number[];
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

// Dashboard statistics types
export interface StatsResponse {
  total_evaluations: number;
  total_batches: number;
  recommendation_counts: RecommendationStats;
  trademark_distribution: ScoreBucket[];
  vice_distribution: ScoreBucket[];
  top_trademark_risks: EvaluationDTO[];
  top_vice_risks: EvaluationDTO[];
  recent_batches: BatchSummary[];
  avg_processing_time_ms: number;
  evaluations_over_time: TimeSeriesPoint[];
}

export interface RecommendationStats {
  block: number;
  review: number;
  allow_with_caution: number;
  allow: number;
}

export interface ScoreBucket {
  score: number;
  count: number;
}

export interface BatchSummary {
  id: number;
  name: string;
  owner: string;
  total_domains: number;
  processed_domains: number;
  block_count: number;
  review_count: number;
  caution_count: number;
  allow_count: number;
  created_at: string;
  last_evaluated_at?: string | null;
}

export interface TimeSeriesPoint {
  date: string;
  count?: number;
  value?: number;
}

// Override types
export interface OverrideRequest {
  overridden_by: string;
  reason: string;
  override_recommendation: string;
  override_explanation?: string;
  override_trademark_score?: number | null;
  override_vice_score?: number | null;
}

export interface OverrideDTO {
  id: number;
  evaluation_id: number;
  domain: string;
  original_trademark_score: number;
  original_vice_score: number;
  original_recommendation: string;
  original_explanation: string;
  override_trademark_score: number | null;
  override_vice_score: number | null;
  override_recommendation: string;
  override_explanation: string;
  overridden_by: string;
  reason: string;
  feedback_applied: boolean;
  created_at: string;
}

export interface OverrideHistoryResponse {
  evaluation: EvaluationDTO;
  overrides: OverrideDTO[];
}

export interface OverridesResponse {
  items: OverrideDTO[];
  total: number;
}

// Feedback types
export interface FeedbackDTO {
  id: number;
  override_id: number;
  domain: string;
  second_level_label: string;
  corrected_recommendation: string;
  corrected_explanation: string;
  corrected_trademark_score: number | null;
  corrected_vice_score: number | null;
  retrieval_count: number;
  last_retrieved_at: string | null;
  created_at: string;
}

export interface FeedbacksResponse {
  items: FeedbackDTO[];
  total: number;
}

// AI Learning Analytics types
export interface AIAccuracyMetrics {
  total_evaluations: number;
  overridden_evaluations: number;
  accuracy_percent: number;
  block_to_allow_rate: number;
  allow_to_block_rate: number;
  average_score_adjustment: number;
}

export interface CorrectionPattern {
  from_recommendation: string;
  to_recommendation: string;
  count: number;
  percentage: number;
}

export interface UserOverrideStats {
  user: string;
  total_overrides: number;
  most_common_change: string;
  last_override_at: string | null;
}

export interface ConfidenceCalibrationBucket {
  confidence_min: number;
  confidence_max: number;
  total_count: number;
  overridden_count: number;
  accuracy_percent: number;
}

export interface FeedbackStatsResponse {
  total_overrides: number;
  total_feedback_embeddings: number;
  override_rate_percent: number;
  feedback_impact_score: number;
  ai_accuracy: AIAccuracyMetrics;
  correction_patterns: CorrectionPattern[];
  user_stats: UserOverrideStats[];
  confidence_calibration: ConfidenceCalibrationBucket[];
  overrides_over_time: TimeSeriesPoint[];
  accuracy_over_time: TimeSeriesPoint[];
}
