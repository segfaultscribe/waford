# waford
 A highly available, low-latency API that receives incoming webhook events from a single source and reliably fans them out to multiple destination APIs asynchronously.

This is a internal micro-tool that confronts webhook that only forward to one endpoint and allows fanning out the the request to multiple endpoints that might need it.
