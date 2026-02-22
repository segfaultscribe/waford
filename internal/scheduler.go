package internal

func scheduler(body []byte, jobId string) {
	// Wrap the payload in a Job
	newJob := Job{
		jobId,
		body,
		0,
	}

	schedule(newJob)
}

func schedule(job Job) {

}
