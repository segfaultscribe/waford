package internal

import (
	"encoding/json"
	"io"
	"net/http"
)

func handleIngress(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)

	if !json.Valid(body) || err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// jobId := uuid.New()
	// // Scheduler()

	// var newJob Job = Job{
	// 	jobId.String(),
	// 	body,
	// 	0,
	// }

}
