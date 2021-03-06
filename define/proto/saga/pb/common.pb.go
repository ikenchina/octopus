// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.21.0
// 	protoc        v3.6.1
// source: common.proto

package pb

import (
	proto "github.com/golang/protobuf/proto"
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

type Saga struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Gtid     string        `protobuf:"bytes,1,opt,name=gtid,proto3" json:"gtid,omitempty"`
	State    string        `protobuf:"bytes,2,opt,name=state,proto3" json:"state,omitempty"`
	Branches []*SagaBranch `protobuf:"bytes,3,rep,name=branches,proto3" json:"branches,omitempty"`
	Msg      string        `protobuf:"bytes,4,opt,name=msg,proto3" json:"msg,omitempty"`
}

func (x *Saga) Reset() {
	*x = Saga{}
	if protoimpl.UnsafeEnabled {
		mi := &file_common_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Saga) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Saga) ProtoMessage() {}

func (x *Saga) ProtoReflect() protoreflect.Message {
	mi := &file_common_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Saga.ProtoReflect.Descriptor instead.
func (*Saga) Descriptor() ([]byte, []int) {
	return file_common_proto_rawDescGZIP(), []int{0}
}

func (x *Saga) GetGtid() string {
	if x != nil {
		return x.Gtid
	}
	return ""
}

func (x *Saga) GetState() string {
	if x != nil {
		return x.State
	}
	return ""
}

func (x *Saga) GetBranches() []*SagaBranch {
	if x != nil {
		return x.Branches
	}
	return nil
}

func (x *Saga) GetMsg() string {
	if x != nil {
		return x.Msg
	}
	return ""
}

type SagaBranch struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BranchId int32  `protobuf:"varint,1,opt,name=branch_id,json=branchId,proto3" json:"branch_id,omitempty"`
	State    string `protobuf:"bytes,2,opt,name=state,proto3" json:"state,omitempty"`
	Payload  []byte `protobuf:"bytes,3,opt,name=payload,proto3" json:"payload,omitempty"`
}

func (x *SagaBranch) Reset() {
	*x = SagaBranch{}
	if protoimpl.UnsafeEnabled {
		mi := &file_common_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SagaBranch) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SagaBranch) ProtoMessage() {}

func (x *SagaBranch) ProtoReflect() protoreflect.Message {
	mi := &file_common_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SagaBranch.ProtoReflect.Descriptor instead.
func (*SagaBranch) Descriptor() ([]byte, []int) {
	return file_common_proto_rawDescGZIP(), []int{1}
}

func (x *SagaBranch) GetBranchId() int32 {
	if x != nil {
		return x.BranchId
	}
	return 0
}

func (x *SagaBranch) GetState() string {
	if x != nil {
		return x.State
	}
	return ""
}

func (x *SagaBranch) GetPayload() []byte {
	if x != nil {
		return x.Payload
	}
	return nil
}

var File_common_proto protoreflect.FileDescriptor

var file_common_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x04,
	0x73, 0x61, 0x67, 0x61, 0x22, 0x70, 0x0a, 0x04, 0x53, 0x61, 0x67, 0x61, 0x12, 0x12, 0x0a, 0x04,
	0x67, 0x74, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x67, 0x74, 0x69, 0x64,
	0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x12, 0x2c, 0x0a, 0x08, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68,
	0x65, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x10, 0x2e, 0x73, 0x61, 0x67, 0x61, 0x2e,
	0x53, 0x61, 0x67, 0x61, 0x42, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x52, 0x08, 0x62, 0x72, 0x61, 0x6e,
	0x63, 0x68, 0x65, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x6d, 0x73, 0x67, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x22, 0x59, 0x0a, 0x0a, 0x53, 0x61, 0x67, 0x61, 0x42, 0x72,
	0x61, 0x6e, 0x63, 0x68, 0x12, 0x1b, 0x0a, 0x09, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x5f, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x08, 0x62, 0x72, 0x61, 0x6e, 0x63, 0x68, 0x49,
	0x64, 0x12, 0x14, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f,
	0x61, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61,
	0x64, 0x42, 0x06, 0x5a, 0x04, 0x2e, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_common_proto_rawDescOnce sync.Once
	file_common_proto_rawDescData = file_common_proto_rawDesc
)

func file_common_proto_rawDescGZIP() []byte {
	file_common_proto_rawDescOnce.Do(func() {
		file_common_proto_rawDescData = protoimpl.X.CompressGZIP(file_common_proto_rawDescData)
	})
	return file_common_proto_rawDescData
}

var file_common_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_common_proto_goTypes = []interface{}{
	(*Saga)(nil),       // 0: saga.Saga
	(*SagaBranch)(nil), // 1: saga.SagaBranch
}
var file_common_proto_depIdxs = []int32{
	1, // 0: saga.Saga.branches:type_name -> saga.SagaBranch
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_common_proto_init() }
func file_common_proto_init() {
	if File_common_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_common_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Saga); i {
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
		file_common_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SagaBranch); i {
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
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_common_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_common_proto_goTypes,
		DependencyIndexes: file_common_proto_depIdxs,
		MessageInfos:      file_common_proto_msgTypes,
	}.Build()
	File_common_proto = out.File
	file_common_proto_rawDesc = nil
	file_common_proto_goTypes = nil
	file_common_proto_depIdxs = nil
}
