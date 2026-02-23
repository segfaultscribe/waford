package internal

import (
	"encoding/json"
	"fmt"
	"os"
)

func (s *Server) handleDLQ() {
	// Open the file in Append mode
	file, err := os.OpenFile("dlq.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("failed to open DLQ file: %v", err))
	}
	defer file.Close()

	for deadJob := range s.jm.DLQBuffer {
		// Convert the struct to a JSON byte array
		jobBytes, err := json.Marshal(deadJob)
		if err != nil {
			fmt.Printf("failed to marshal dead job: %v\n", err)
			continue
		}
		// Append to the file with a newline
		file.Write(jobBytes)
		file.WriteString("\n")

		s.logger.Warn("Job moved to DLQ", "event_id", deadJob.EventID)
	}
}
