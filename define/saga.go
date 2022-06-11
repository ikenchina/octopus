package define

import (
	"time"
)

type SagaRequest struct {
	NodeId            int `json:"-"`
	DcId              int `json:"-"`
	Gtid              string
	Business          string
	Notify            *SagaNotify
	ExpireTime        time.Time
	SagaCallType      string // sync or async
	Branches          []SagaBranch
	ParallelExecution bool
	Lessee            string `json:"-"`
}

type SagaNotify struct {
	Action  string
	Timeout time.Duration
	Retry   time.Duration
}

type SagaBranchCommit struct {
	Action  string
	Timeout time.Duration
	Retry   SagaRetry
}

type SagaRetry struct {
	MaxRetry int
	Constant *RetryStrategyConstant
}

type SagaBranchCompensation struct {
	Action  string
	Timeout time.Duration
	// @todo retry strategy
	Retry time.Duration
}

type SagaBranch struct {
	BranchId     int
	Payload      string
	Commit       SagaBranchCommit
	Compensation SagaBranchCompensation
}

type RetryStrategyConstant struct {
	Duration time.Duration
}

type SagaResponse struct {
	Gtid     string
	State    string
	Branches []SagaBranchResponse
	Msg      string
}

type SagaBranchResponse struct {
	BranchId int
	State    string
	Payload  string
}
