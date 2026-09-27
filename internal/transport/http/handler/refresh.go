package handler

import (
	"context"
	"net/http"
	"time"

	"search-engine/internal/ingestion"
)

type RefreshHandler struct {
	job *ingestion.Job
}

func NewRefreshHandler(job *ingestion.Job) *RefreshHandler {
	return &RefreshHandler{job: job}
}

// refreshTimeout is the upper bound given to the job. It is used INSTEAD
// OF the HTTP request's context — otherwise, if the client cut the
// connection (a curl timeout, a closed browser tab, a reverse proxy
// timeout), the entire in-progress provider fetch/write operation would
// also be canceled instantly; this actually happened while writing a
// provider with thousands of records (context canceled). We deliberately
// decouple the job from the request's lifecycle.
const refreshTimeout = 2 * time.Minute

// ServeHTTP handles a POST /refresh or POST /refresh?provider=xxx
// request. If no provider is specified, all providers are refreshed. The
// job runs synchronously (acceptable within the scope of this case
// study); in real production this would drop a job onto a message queue
// (SQS/RabbitMQ) and return 202 Accepted — at that point this context
// problem would also disappear on its own, since the job would already
// run in a worker independent of the HTTP request.
func (h *RefreshHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	if name := r.URL.Query().Get("provider"); name != "" {
		if err := h.job.RunOne(ctx, name); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed", "provider": name})
		return
	}
	h.job.RunAll(ctx)
	writeJSON(w, http.StatusOK, map[string]string{"status": "all providers refreshed"})
}
