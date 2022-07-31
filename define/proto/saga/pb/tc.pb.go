// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.21.0
// 	protoc        v3.6.1
// source: tc.proto

package pb

import (
	proto "github.com/golang/protobuf/proto"
	duration "github.com/golang/protobuf/ptypes/duration"
	empty "github.com/golang/protobuf/ptypes/empty"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type SagaRequest_CallType int32

const (
	SagaRequest_SYNC  SagaRequest_CallType = 0
	SagaRequest_ASYNC SagaRequest_CallType = 1
)

// Enum value maps for SagaRequest_CallType.
var (
	SagaRequest_CallType_name = map[int32]string{
		0: "SYNC",
		1: "ASYNC",
	}
	SagaRequest_CallType_value = map[string]int32{
		"SYNC":  0,
		"ASYNC": 1,
	}
)

func (x SagaRequest_CallType) Enum() *SagaRequest_CallType {
	p := new(SagaRequest_CallType)
	*p = x
	return p
}

func (x SagaRequest_CallType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (SagaRequest_CallType) Descriptor() protoreflect.EnumDescriptor {
	return file_tc_proto_enumTypes[0].Descriptor()
}

func (SagaRequest_CallType) Type() protoreflect.EnumType {
	return &file_tc_proto_enumTypes[0]
}

func (x SagaRequest_CallType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use SagaRequest_CallType.Descriptor instead.
func (SagaRequest_CallType) EnumDescriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{0, 0}
}

type SagaRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Gtid       string               `protobuf:"bytes,1,opt,name=gtid,proto3" json:"gtid,omitempty"`
	Business   string               `protobuf:"bytes,2,opt,name=business,proto3" json:"business,omitempty"`
	Notify     *SagaNotify          `protobuf:"bytes,3,opt,name=notify,proto3" json:"notify,omitempty"`
	ExpireTime *timestamp.Timestamp `protobuf:"bytes,4,opt,name=expire_time,json=expireTime,proto3" json:"expire_time,omitempty"`
	CallType   SagaRequest_CallType `protobuf:"varint,5,opt,name=call_type,json=callType,proto3,enum=saga.SagaRequest_CallType" json:"call_type,omitempty"`
	Branches   []*SagaBranchRequest `protobuf:"bytes,6,rep,name=branches,proto3" json:"branches,omitempty"`
}

func (x *SagaRequest) Reset() {
	*x = SagaRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaRequest) ProtoMessage() {}

func (x *SagaRequest) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaRequest.ProtoReflect.Descriptor instead.
func (*SagaRequest) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{0}
}

func (x *SagaRequest) GetGtid() string {
	if x != nil {
		return x.Gtid
	}
	return ""
}

func (x *SagaRequest) GetBusiness() string {
	if x != nil {
		return x.Business
	}
	return ""
}

func (x *SagaRequest) GetNotify() *SagaNotify {
	if x != nil {
		return x.Notify
	}
	return nil
}

func (x *SagaRequest) GetExpireTime() *timestamp.Timestamp {
	if x != nil {
		return x.ExpireTime
	}
	return nil
}

func (x *SagaRequest) GetCallType() SagaRequest_CallType {
	if x != nil {
		return x.CallType
	}
	return SagaRequest_SYNC
}

func (x *SagaRequest) GetBranches() []*SagaBranchRequest {
	if x != nil {
		return x.Branches
	}
	return nil
}

type SagaBranchRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BranchId     int32                           `protobuf:"varint,1,opt,name=branch_id,json=branchId,proto3" json:"branch_id,omitempty"`
	Commit       *SagaBranchRequest_Commit       `protobuf:"bytes,2,opt,name=commit,proto3" json:"commit,omitempty"`
	Compensation *SagaBranchRequest_Compensation `protobuf:"bytes,3,opt,name=compensation,proto3" json:"compensation,omitempty"`
	Payload      []byte                          `protobuf:"bytes,4,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *SagaBranchRequest) Reset() {
	*x = SagaBranchRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaBranchRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaBranchRequest) ProtoMessage() {}

func (x *SagaBranchRequest) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaBranchRequest.ProtoReflect.Descriptor instead.
func (*SagaBranchRequest) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{1}
}

func (x *SagaBranchRequest) GetBranchId() int32 {
	if x != nil {
		return x.BranchId
	}
	return 0
}

func (x *SagaBranchRequest) GetCommit() *SagaBranchRequest_Commit {
	if x != nil {
		return x.Commit
	}
	return nil
}

func (x *SagaBranchRequest) GetCompensation() *SagaBranchRequest_Compensation {
	if x != nil {
		return x.Compensation
	}
	return nil
}

func (x *SagaBranchRequest) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

type SagaNotify struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Action  string             `protobuf:"bytes,1,opt,name=action,proto3" json:"action,omitempty"`
	Timeout *duration.Duration `protobuf:"bytes,2,opt,name=timeout,proto3" json:"timeout,omitempty"`
	Retry   *duration.Duration `protobuf:"bytes,3,opt,name=retry,proto3" json:"retry,omitempty"`
}

func (x *SagaNotify) Reset() {
	*x = SagaNotify{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaNotify) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaNotify) ProtoMessage() {}

func (x *SagaNotify) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaNotify.ProtoReflect.Descriptor instead.
func (*SagaNotify) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{2}
}

func (x *SagaNotify) GetAction() string {
	if x != nil {
		return x.Action
	}
	return ""
}

func (x *SagaNotify) GetTimeout() *duration.Duration {
	if x != nil {
		return x.Timeout
	}
	return nil
}

func (x *SagaNotify) GetRetry() *duration.Duration {
	if x != nil {
		return x.Retry
	}
	return nil
}

type SagaResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Saga *Saga `protobuf:"bytes,1,opt,name=saga,proto3" json:"saga,omitempty"`
}

func (x *SagaResponse) Reset() {
	*x = SagaResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaResponse) ProtoMessage() {}

func (x *SagaResponse) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaResponse.ProtoReflect.Descriptor instead.
func (*SagaResponse) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{3}
}

func (x *SagaResponse) GetSaga() *Saga {
	if x != nil {
		return x.Saga
	}
	return nil
}

type SagaBranchRequest_RetryStrategy struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Strategy:
	//	*SagaBranchRequest_RetryStrategy_Constant
	Strategy isSagaBranchRequest_RetryStrategy_Strategy `protobuf_oneof:"strategy"`
}

func (x *SagaBranchRequest_RetryStrategy) Reset() {
	*x = SagaBranchRequest_RetryStrategy{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaBranchRequest_RetryStrategy) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaBranchRequest_RetryStrategy) ProtoMessage() {}

func (x *SagaBranchRequest_RetryStrategy) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaBranchRequest_RetryStrategy.ProtoReflect.Descriptor instead.
func (*SagaBranchRequest_RetryStrategy) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{1, 0}
}

func (m *SagaBranchRequest_RetryStrategy) GetStrategy() isSagaBranchRequest_RetryStrategy_Strategy {
	if m != nil {
		return m.Strategy
	}
	return nil
}

func (x *SagaBranchRequest_RetryStrategy) GetConstant() *duration.Duration {
	if x, ok := x.GetStrategy().(*SagaBranchRequest_RetryStrategy_Constant); ok {
		return x.Constant
	}
	return nil
}

type isSagaBranchRequest_RetryStrategy_Strategy interface {
	isSagaBranchRequest_RetryStrategy_Strategy()
}

type SagaBranchRequest_RetryStrategy_Constant struct {
	Constant *duration.Duration `protobuf:"bytes,1,opt,name=constant,proto3,oneof"`
}

func (*SagaBranchRequest_RetryStrategy_Constant) isSagaBranchRequest_RetryStrategy_Strategy() {}

type SagaBranchRequest_Retry struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	MaxRetry int32                            `protobuf:"varint,1,opt,name=max_retry,json=maxRetry,proto3" json:"max_retry,omitempty"`
	Strategy *SagaBranchRequest_RetryStrategy `protobuf:"bytes,2,opt,name=strategy,proto3" json:"strategy,omitempty"`
}

func (x *SagaBranchRequest_Retry) Reset() {
	*x = SagaBranchRequest_Retry{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaBranchRequest_Retry) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaBranchRequest_Retry) ProtoMessage() {}

func (x *SagaBranchRequest_Retry) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaBranchRequest_Retry.ProtoReflect.Descriptor instead.
func (*SagaBranchRequest_Retry) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{1, 1}
}

func (x *SagaBranchRequest_Retry) GetMaxRetry() int32 {
	if x != nil {
		return x.MaxRetry
	}
	return 0
}

func (x *SagaBranchRequest_Retry) GetStrategy() *SagaBranchRequest_RetryStrategy {
	if x != nil {
		return x.Strategy
	}
	return nil
}

type SagaBranchRequest_Commit struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Action  string                   `protobuf:"bytes,1,opt,name=action,proto3" json:"action,omitempty"`
	Timeout *duration.Duration       `protobuf:"bytes,2,opt,name=timeout,proto3" json:"timeout,omitempty"`
	Retry   *SagaBranchRequest_Retry `protobuf:"bytes,3,opt,name=retry,proto3" json:"retry,omitempty"`
}

func (x *SagaBranchRequest_Commit) Reset() {
	*x = SagaBranchRequest_Commit{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaBranchRequest_Commit) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaBranchRequest_Commit) ProtoMessage() {}

func (x *SagaBranchRequest_Commit) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaBranchRequest_Commit.ProtoReflect.Descriptor instead.
func (*SagaBranchRequest_Commit) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{1, 2}
}

func (x *SagaBranchRequest_Commit) GetAction() string {
	if x != nil {
		return x.Action
	}
	return ""
}

func (x *SagaBranchRequest_Commit) GetTimeout() *duration.Duration {
	if x != nil {
		return x.Timeout
	}
	return nil
}

func (x *SagaBranchRequest_Commit) GetRetry() *SagaBranchRequest_Retry {
	if x != nil {
		return x.Retry
	}
	return nil
}

type SagaBranchRequest_Compensation struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Action  string             `protobuf:"bytes,1,opt,name=action,proto3" json:"action,omitempty"`
	Timeout *duration.Duration `protobuf:"bytes,2,opt,name=timeout,proto3" json:"timeout,omitempty"`
	Retry   *duration.Duration `protobuf:"bytes,3,opt,name=retry,proto3" json:"retry,omitempty"`
}

func (x *SagaBranchRequest_Compensation) Reset() {
	*x = SagaBranchRequest_Compensation{}
	if protoimpl.UnsafeEnabled {
		mi := &file_tc_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaBranchRequest_Compensation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaBranchRequest_Compensation) ProtoMessage() {}

func (x *SagaBranchRequest_Compensation) ProtoReflect() protoreflect.Message {
	mi := &file_tc_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaBranchRequest_Compensation.ProtoReflect.Descriptor instead.
func (*SagaBranchRequest_Compensation) Descriptor() ([]byte, []int) {
	return file_tc_proto_rawDescGZIP(), []int{1, 3}
}

func (x *SagaBranchRequest_Compensation) GetAction() string {
	if x != nil {
		return x.Action
	}
	return ""
}

func (x *SagaBranchRequest_Compensation) GetTimeout() *duration.Duration {
	if x != nil {
		return x.Timeout
	}
	return nil
}

func (x *SagaBranchRequest_Compensation) GetRetry() *duration.Duration {
	if x != nil {
		return x.Retry
	}
	return nil
}

var File_tc_proto protoreflect.FileDescriptor

var file_tc_proto_rawDesc = []byte{
	0x0a, 0x08, 0x74, 0x63, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x04, 0x73, 0x61, 0x67, 0x61,
	0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64,
	0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74,
	0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x0c,
	0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb3, 0x02, 0x0a,
	0x0b, 0x53, 0x61, 0x67, 0x61, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04,
	0x67, 0x74, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x67, 0x74, 0x69, 0x64,
	0x12, 0x1a, 0x0a, 0x08, 0x62, 0x75, 0x73, 0x69, 0x6e, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x08, 0x62, 0x75, 0x73, 0x69, 0x6e, 0x65, 0x73, 0x73, 0x12, 0x28, 0x0a, 0x06,
	0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x73,
	0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x4e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x52, 0x06,
	0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x12, 0x3b, 0x0a, 0x0b, 0x65, 0x78, 0x70, 0x69, 0x72, 0x65,
	0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69,
	0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x0a, 0x65, 0x78, 0x70, 0x69, 0x72, 0x65, 0x54,
	0x69, 0x6d, 0x65, 0x12, 0x37, 0x0a, 0x09, 0x63, 0x61, 0x6c, 0x6c, 0x5f, 0x74, 0x79, 0x70, 0x65,
	0x18, 0x05, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x1a, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61,
	0x67, 0x61, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x43, 0x61, 0x6c, 0x6c, 0x54, 0x79,
	0x70, 0x65, 0x52, 0x08, 0x63, 0x61, 0x6c, 0x6c, 0x54, 0x79, 0x70, 0x65, 0x12, 0x33, 0x0a, 0x08,
	0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x17,
	0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x42, 0x72, 0x61, 0x6e, 0x63, 0x68,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x52, 0x08, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x65,
	0x73, 0x22, 0x1f, 0x0a, 0x08, 0x43, 0x61, 0x6c, 0x6c, 0x54, 0x79, 0x70, 0x65, 0x12, 0x08, 0x0a,
	0x04, 0x53, 0x59, 0x4e, 0x43, 0x10, 0x00, 0x12, 0x09, 0x0a, 0x05, 0x41, 0x53, 0x59, 0x4e, 0x43,
	0x10, 0x01, 0x22, 0xa7, 0x05, 0x0a, 0x11, 0x53, 0x61, 0x67, 0x61, 0x42, 0x72, 0x61, 0x6e, 0x63,
	0x68, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1b, 0x0a, 0x09, 0x62, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x08, 0x62, 0x72, 0x61,
	0x6e, 0x63, 0x68, 0x49, 0x64, 0x12, 0x36, 0x0a, 0x06, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67,
	0x61, 0x42, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x43,
	0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x52, 0x06, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x12, 0x48, 0x0a,
	0x0c, 0x63, 0x6f, 0x6d, 0x70, 0x65, 0x6e, 0x73, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x24, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x42,
	0x72, 0x61, 0x6e, 0x63, 0x68, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x43, 0x6f, 0x6d,
	0x70, 0x65, 0x6e, 0x73, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0c, 0x63, 0x6f, 0x6d, 0x70, 0x65,
	0x6e, 0x73, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f,
	0x61, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61,
	0x64, 0x1a, 0x54, 0x0a, 0x0d, 0x52, 0x65, 0x74, 0x72, 0x79, 0x53, 0x74, 0x72, 0x61, 0x74, 0x65,
	0x67, 0x79, 0x12, 0x37, 0x0a, 0x08, 0x63, 0x6f, 0x6e, 0x73, 0x74, 0x61, 0x6e, 0x74, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x48,
	0x00, 0x52, 0x08, 0x63, 0x6f, 0x6e, 0x73, 0x74, 0x61, 0x6e, 0x74, 0x42, 0x0a, 0x0a, 0x08, 0x73,
	0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x1a, 0x67, 0x0a, 0x05, 0x52, 0x65, 0x74, 0x72, 0x79,
	0x12, 0x1b, 0x0a, 0x09, 0x6d, 0x61, 0x78, 0x5f, 0x72, 0x65, 0x74, 0x72, 0x79, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x05, 0x52, 0x08, 0x6d, 0x61, 0x78, 0x52, 0x65, 0x74, 0x72, 0x79, 0x12, 0x41, 0x0a,
	0x08, 0x73, 0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x25, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x42, 0x72, 0x61, 0x6e, 0x63,
	0x68, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x52, 0x65, 0x74, 0x72, 0x79, 0x53, 0x74,
	0x72, 0x61, 0x74, 0x65, 0x67, 0x79, 0x52, 0x08, 0x73, 0x74, 0x72, 0x61, 0x74, 0x65, 0x67, 0x79,
	0x1a, 0x8a, 0x01, 0x0a, 0x06, 0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x61,
	0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x61, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x12, 0x33, 0x0a, 0x07, 0x74, 0x69, 0x6d, 0x65, 0x6f, 0x75, 0x74, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52,
	0x07, 0x74, 0x69, 0x6d, 0x65, 0x6f, 0x75, 0x74, 0x12, 0x33, 0x0a, 0x05, 0x72, 0x65, 0x74, 0x72,
	0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53,
	0x61, 0x67, 0x61, 0x42, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x2e, 0x52, 0x65, 0x74, 0x72, 0x79, 0x52, 0x05, 0x72, 0x65, 0x74, 0x72, 0x79, 0x1a, 0x8c, 0x01,
	0x0a, 0x0c, 0x43, 0x6f, 0x6d, 0x70, 0x65, 0x6e, 0x73, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x16,
	0x0a, 0x06, 0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x61, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x33, 0x0a, 0x07, 0x74, 0x69, 0x6d, 0x65, 0x6f, 0x75,
	0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x07, 0x74, 0x69, 0x6d, 0x65, 0x6f, 0x75, 0x74, 0x12, 0x2f, 0x0a, 0x05, 0x72,
	0x65, 0x74, 0x72, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x05, 0x72, 0x65, 0x74, 0x72, 0x79, 0x22, 0x8a, 0x01, 0x0a,
	0x0a, 0x53, 0x61, 0x67, 0x61, 0x4e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x12, 0x16, 0x0a, 0x06, 0x61,
	0x63, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x61, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x12, 0x33, 0x0a, 0x07, 0x74, 0x69, 0x6d, 0x65, 0x6f, 0x75, 0x74, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52,
	0x07, 0x74, 0x69, 0x6d, 0x65, 0x6f, 0x75, 0x74, 0x12, 0x2f, 0x0a, 0x05, 0x72, 0x65, 0x74, 0x72,
	0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x05, 0x72, 0x65, 0x74, 0x72, 0x79, 0x22, 0x2e, 0x0a, 0x0c, 0x53, 0x61, 0x67,
	0x61, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1e, 0x0a, 0x04, 0x73, 0x61, 0x67,
	0x61, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0a, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53,
	0x61, 0x67, 0x61, 0x52, 0x04, 0x73, 0x61, 0x67, 0x61, 0x32, 0xa0, 0x01, 0x0a, 0x02, 0x54, 0x63,
	0x12, 0x37, 0x0a, 0x07, 0x4e, 0x65, 0x77, 0x47, 0x74, 0x69, 0x64, 0x12, 0x16, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d,
	0x70, 0x74, 0x79, 0x1a, 0x12, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x31, 0x0a, 0x06, 0x43, 0x6f, 0x6d,
	0x6d, 0x69, 0x74, 0x12, 0x11, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61,
	0x67, 0x61, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x2e, 0x0a, 0x03,
	0x47, 0x65, 0x74, 0x12, 0x11, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61, 0x67, 0x61, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e, 0x53, 0x61,
	0x67, 0x61, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x06, 0x5a, 0x04,
	0x2e, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_tc_proto_rawDescOnce sync.Once
	file_tc_proto_rawDescData = file_tc_proto_rawDesc
)

func file_tc_proto_rawDescGZIP() []byte {
	file_tc_proto_rawDescOnce.Do(func() {
		file_tc_proto_rawDescData = protoimpl.X.CompressGZIP(file_tc_proto_rawDescData)
	})
	return file_tc_proto_rawDescData
}

var file_tc_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_tc_proto_msgTypes = make([]protoimpl.MessageInfo, 8)
var file_tc_proto_goTypes = []interface{}{
	(SagaRequest_CallType)(0),               // 0: saga.SagaRequest.CallType
	(*SagaRequest)(nil),                     // 1: saga.SagaRequest
	(*SagaBranchRequest)(nil),               // 2: saga.SagaBranchRequest
	(*SagaNotify)(nil),                      // 3: saga.SagaNotify
	(*SagaResponse)(nil),                    // 4: saga.SagaResponse
	(*SagaBranchRequest_RetryStrategy)(nil), // 5: saga.SagaBranchRequest.RetryStrategy
	(*SagaBranchRequest_Retry)(nil),         // 6: saga.SagaBranchRequest.Retry
	(*SagaBranchRequest_Commit)(nil),        // 7: saga.SagaBranchRequest.Commit
	(*SagaBranchRequest_Compensation)(nil),  // 8: saga.SagaBranchRequest.Compensation
	(*timestamp.Timestamp)(nil),             // 9: google.protobuf.Timestamp
	(*duration.Duration)(nil),               // 10: google.protobuf.Duration
	(*Saga)(nil),                            // 11: saga.Saga
	(*empty.Empty)(nil),                     // 12: google.protobuf.Empty
}
var file_tc_proto_depIdxs = []int32{
	3,  // 0: saga.SagaRequest.notify:type_name -> saga.SagaNotify
	9,  // 1: saga.SagaRequest.expire_time:type_name -> google.protobuf.Timestamp
	0,  // 2: saga.SagaRequest.call_type:type_name -> saga.SagaRequest.CallType
	2,  // 3: saga.SagaRequest.branches:type_name -> saga.SagaBranchRequest
	7,  // 4: saga.SagaBranchRequest.commit:type_name -> saga.SagaBranchRequest.Commit
	8,  // 5: saga.SagaBranchRequest.compensation:type_name -> saga.SagaBranchRequest.Compensation
	10, // 6: saga.SagaNotify.timeout:type_name -> google.protobuf.Duration
	10, // 7: saga.SagaNotify.retry:type_name -> google.protobuf.Duration
	11, // 8: saga.SagaResponse.saga:type_name -> saga.Saga
	10, // 9: saga.SagaBranchRequest.RetryStrategy.constant:type_name -> google.protobuf.Duration
	5,  // 10: saga.SagaBranchRequest.Retry.strategy:type_name -> saga.SagaBranchRequest.RetryStrategy
	10, // 11: saga.SagaBranchRequest.Commit.timeout:type_name -> google.protobuf.Duration
	6,  // 12: saga.SagaBranchRequest.Commit.retry:type_name -> saga.SagaBranchRequest.Retry
	10, // 13: saga.SagaBranchRequest.Compensation.timeout:type_name -> google.protobuf.Duration
	10, // 14: saga.SagaBranchRequest.Compensation.retry:type_name -> google.protobuf.Duration
	12, // 15: saga.Tc.NewGtid:input_type -> google.protobuf.Empty
	1,  // 16: saga.Tc.Commit:input_type -> saga.SagaRequest
	1,  // 17: saga.Tc.Get:input_type -> saga.SagaRequest
	4,  // 18: saga.Tc.NewGtid:output_type -> saga.SagaResponse
	4,  // 19: saga.Tc.Commit:output_type -> saga.SagaResponse
	4,  // 20: saga.Tc.Get:output_type -> saga.SagaResponse
	18, // [18:21] is the sub-list for method output_type
	15, // [15:18] is the sub-list for method input_type
	15, // [15:15] is the sub-list for extension type_name
	15, // [15:15] is the sub-list for extension extendee
	0,  // [0:15] is the sub-list for field type_name
}

func init() { file_tc_proto_init() }
func file_tc_proto_init() {
	if File_tc_proto != nil {
		return
	}
	file_common_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_tc_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaBranchRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaNotify); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaBranchRequest_RetryStrategy); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaBranchRequest_Retry); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaBranchRequest_Commit); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_tc_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaBranchRequest_Compensation); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_tc_proto_msgTypes[4].OneofWrappers = []interface{}{
		(*SagaBranchRequest_RetryStrategy_Constant)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_tc_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   8,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_tc_proto_goTypes,
		DependencyIndexes: file_tc_proto_depIdxs,
		EnumInfos:         file_tc_proto_enumTypes,
		MessageInfos:      file_tc_proto_msgTypes,
	}.Build()
	File_tc_proto = out.File
	file_tc_proto_rawDesc = nil
	file_tc_proto_goTypes = nil
	file_tc_proto_depIdxs = nil
}
