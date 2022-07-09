package sgrpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/protobuf/proto"
)

func init() {
	encoding.RegisterCodec(customPbCodec{})
}

// @todo cache clients
func Invoke(ctx context.Context, domain string, method string, payload string) ([]byte, error) {
	cc, err := grpc.Dial(domain,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	out := []byte{}
	err = grpc.Invoke(ctx, method, []byte(payload), &out, cc)
	return out, err
}

type customPbCodec struct {
}

func (c customPbCodec) Name() string {
	return "proto"
}
func (c customPbCodec) Marshal(v interface{}) ([]byte, error) {

	switch v.(type) {
	case []byte:
		return v.([]byte), nil
	}
	return proto.Marshal(v.(proto.Message))
}

func (c customPbCodec) Unmarshal(data []byte, v interface{}) error {
	switch v.(type) {
	case *[]byte:
		vv := v.(*[]byte)
		*vv = data
	default:
		return proto.Unmarshal(data, v.(proto.Message))
	}
	return nil
}
