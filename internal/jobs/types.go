package jobs

import "errors"

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

var ErrJobAlreadyActive = errors.New("活动任务已存在")
