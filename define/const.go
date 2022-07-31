package define

const (
	TxnTypeSaga = "saga"
	TxnTypeTcc  = "tcc"

	BranchTypeCommit       = "commit"
	BranchTypeCompensation = "compensation"
	BranchTypeConfirm      = "confirm"
	BranchTypeCancel       = "cancel"

	//
	TxnStatePrepared     = "prepared"
	TxnStateCommitting   = "committing"
	TxnStatePreCommitted = "precommitted" // notifying
	TxnStateCommitted    = "committed"
	TxnStateRolling      = "rolling"
	TxnStatePreAborted   = "preaborted" // notifying
	TxnStateAborted      = "aborted"

	TxnCallTypeSync  = "sync"
	TxnCallTypeAsync = "async"

	RmProtocolGrpc     = "grpc"
	RmProtocolSeparate = ";"

	PayloadCodecJson  = "json"
	PayloadCodecProto = "proto"
)
