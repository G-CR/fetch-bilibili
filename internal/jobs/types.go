package jobs

const (
	TypeFetch    = "fetch"
	TypeDownload = "download"
	TypeCheck    = "check"
	TypeCleanup  = "cleanup"
)

const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)
