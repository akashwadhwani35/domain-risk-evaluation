package api

import (
	"encoding/json"
	"strings"

	"github.com/sirupsen/logrus"

	"domain-risk-eval/backend/internal/store"
)

func (s *Server) broadcastJobEvent(job *evaluationJob, event EvaluationEvent) {
	s.evalNotifier.Broadcast(event)
	s.persistJobState(job, event)
}

func (s *Server) persistJobState(job *evaluationJob, event EvaluationEvent) {
	if s == nil || s.db == nil {
		return
	}
	jobID := strings.TrimSpace(event.JobID)
	if jobID == "" && job != nil {
		jobID = job.id
	}
	if jobID == "" {
		return
	}
	batchID := event.BatchID
	if batchID == 0 && job != nil {
		batchID = job.batchID
	}
	requestID := uint(0)
	if job != nil {
		requestID = job.requestID
	}
	payload, err := json.Marshal(event)
	if err != nil {
		logrus.WithError(err).Warn("marshal job state event")
		return
	}
	state := &store.JobState{
		JobID:         jobID,
		BatchID:       batchID,
		RequestID:     requestID,
		Status:        event.Type,
		Message:       strings.TrimSpace(event.Message),
		Processed:     event.Processed,
		Total:         event.Total,
		LastEventJSON: string(payload),
	}
	if err := s.db.UpsertJobState(state); err != nil {
		logrus.WithError(err).Warn("persist job state")
	}
}

