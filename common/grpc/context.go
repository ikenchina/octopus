package sgrpc

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"
)

const (
	GRPC_HREADER_GTID         = "DTX_GTID"
	GRPC_HREADER_BRANCHID     = "DTX_BRANCH_ID"
	GRPC_HEADER_TC_DATACENTER = "DTX_TC_DATACENTER"
	GRPC_HEADER_TC_NODE       = "DTX_TC_NODE"
	GRPC_HEADER_TXN_TYPE      = "DTX_TXN_TYPE"
)

func ParseContextMeta(ctx context.Context) (gtid string, bid int) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return
	}

	if gtids := md.Get(GRPC_HREADER_GTID); len(gtids) > 0 {
		gtid = gtids[0]
		if bids := md.Get(GRPC_HREADER_BRANCHID); len(bids) > 0 {
			bid, _ = strconv.Atoi(bids[0])
		}
	}
	return
}

func SetMetaFromOutgoingContext(ctx context.Context, gtid string, bid int) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}
	md.Set(GRPC_HREADER_GTID, gtid)
	md.Set(GRPC_HREADER_BRANCHID, strconv.Itoa(bid))
	return metadata.NewOutgoingContext(ctx, md)
}
