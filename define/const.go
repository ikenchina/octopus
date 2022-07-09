package define

const (
	TxnTypeSaga = "saga"
	TxnTypeTcc  = "tcc"

	BranchTypeCommit       = "commit"
	BranchTypeCompensation = "compensation"
	BranchTypeConfirm      = "confirm"
	BranchTypeCancel       = "cancel"

	//
	TxnStatePrepared   = "prepared"
	TxnStateFailed     = "failed"
	TxnStateAborted    = "aborted"
	TxnStateCommitting = "committing"
	TxnStateCommitted  = "committed"

	TxnCallTypeSync  = "sync"
	TxnCallTypeAsync = "async"

	RmProtocolGrpc     = "grpc"
	RmProtocolSeparate = ";"
)
