package define

import (
	"time"
)

type TccRequest struct {
	NodeId     int `json:"-"`
	DcId       int `json:"-"`
	Gtid       string
	Business   string
	ExpireTime time.Time
	Branches   []TccBranch
	Lessee     string `json:"-"`
}

type TccBranch struct {
	BranchId      int
	ActionConfirm string
	ActionCancel  string
	Payload       string
	Timeout       time.Duration
	Retry         time.Duration
}

type TccResponse struct {
	Gtid     string
	State    string
	Branches []TccBranchResponse
	Msg      string
}

type TccBranchResponse struct {
	BranchId int
	State    string
}
