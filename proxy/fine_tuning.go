package proxy

import "net/http"

// OpenAI Fine-tuning Jobs API — pure openai-family passthrough (#119).
//
//	POST   /v1/fine_tuning/jobs
//	GET    /v1/fine_tuning/jobs
//	GET    /v1/fine_tuning/jobs/{job_id}
//	POST   /v1/fine_tuning/jobs/{job_id}/cancel
//	GET    /v1/fine_tuning/jobs/{job_id}/events
//	GET    /v1/fine_tuning/jobs/{job_id}/checkpoints

func (s *Server) handleFineTuningJobsCreate(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodPost, "/fine_tuning/jobs", true)
}

func (s *Server) handleFineTuningJobsList(w http.ResponseWriter, r *http.Request) {
	s.openAIFamilyProxy(w, r, http.MethodGet, "/fine_tuning/jobs", false)
}

func (s *Server) handleFineTuningJobsGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing job id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodGet, "/fine_tuning/jobs/"+id, false)
}

func (s *Server) handleFineTuningJobsCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing job id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodPost, "/fine_tuning/jobs/"+id+"/cancel", false)
}

func (s *Server) handleFineTuningJobsEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing job id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodGet, "/fine_tuning/jobs/"+id+"/events", false)
}

func (s *Server) handleFineTuningJobsCheckpoints(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing job id")
		return
	}
	s.openAIFamilyProxy(w, r, http.MethodGet, "/fine_tuning/jobs/"+id+"/checkpoints", false)
}
