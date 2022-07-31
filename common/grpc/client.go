package sgrpc

import (
	"context"
	"strconv"

	consistentbalancer "github.com/authzed/spicedb/pkg/balancer"
	"github.com/cespare/xxhash"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	lru "github.com/hashicorp/golang-lru"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func RegisterCustomProto() {
	encoding.RegisterCodec(customPbCodec{})
}

var (
	connPools *lru.Cache
)

func Init(size int) error {
	balancer.Register(consistentbalancer.NewConsistentHashringBuilder(xxhash.Sum64, 10, 1))

	if size <= 0 {
		size = 1000
	}
	cp, err := lru.NewWithEvict(size, func(key interface{}, value interface{}) {
		con := value.(*grpc.ClientConn)
		con.Close()
	})
	if err != nil {
		return err
	}
	connPools = cp
	return nil
}

func unaryClientInterceptor(opts0 ...grpc.CallOption) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		opts = append(opts, opts0...)
		err := invoker(ctx, method, req, reply, cc, opts...)
		return err
	}
}

func getGrpcConn(domain string) (*grpc.ClientConn, error) {
	val, ok := connPools.Get(domain)
	if !ok {
		dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
		unaryMiddlwares := []grpc.UnaryClientInterceptor{unaryClientInterceptor(grpc.WaitForReady(false))}
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(unaryMiddlwares...)))
		dialOptions = append(dialOptions, grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"consistent-hashring"}`))

		cc, err := grpc.Dial(domain, dialOptions...)
		if err != nil {
			return nil, err
		}

		c2, ok, _ := connPools.PeekOrAdd(domain, cc)
		if ok {
			cc.Close()
			val = c2
		} else {
			val = cc
		}
	}

	return val.(*grpc.ClientConn), nil
}

func Invoke(ctx context.Context, domain string, gtid string,
	bid int, method string, payload []byte) ([]byte, error) {

	ctx = context.WithValue(ctx, consistentbalancer.CtxKey, []byte(gtid))
	cc, err := getGrpcConn(domain)
	if err != nil {
		return nil, err
	}

	out := []byte{}
	in := payload
	ctx = metadata.AppendToOutgoingContext(ctx, GRPC_HREADER_GTID, gtid, GRPC_HREADER_BRANCHID, strconv.Itoa(bid))
	err = grpc.Invoke(ctx, method, in, &out, cc, grpc.WaitForReady(true))
	return out, err
}

type customPbCodec struct {
}

func (c customPbCodec) Name() string {
	return "proto"
}

func (c customPbCodec) Marshal(v interface{}) ([]byte, error) {
	switch tv := v.(type) {
	case []byte:
		return tv, nil
	default:
		return proto.Marshal(v.(proto.Message))
	}
}

func (c customPbCodec) Unmarshal(data []byte, v interface{}) error {
	switch tv := v.(type) {
	case *[]byte:
		vv := tv
		*vv = data
	default:
		return proto.Unmarshal(data, v.(proto.Message))
	}
	return nil
}
