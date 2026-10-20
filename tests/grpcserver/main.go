// Independent gRPC peer for tarn-grpc integration: real grpc-go over
// plaintext HTTP/2. A raw-bytes codec replaces generated code; messages are
// decoded with protowire, an implementation independent of tarn-protobuf.
// Listens on 127.0.0.1:0 and prints the address on its first line.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

type raw struct{}

func (raw) Marshal(v any) ([]byte, error) { return *(v.(*[]byte)), nil }
func (raw) Unmarshal(data []byte, v any) error {
	*(v.(*[]byte)) = append([]byte(nil), data...)
	return nil
}
func (raw) Name() string { return "proto" }

// Sum request: repeated field 1 (varint) and field 2 (string label).
// Response: field 1 sum (varint), field 2 label echoed, field 3 sint64 -sum.
func sum(data []byte) ([]byte, error) {
	var total uint64
	var label []byte
	for len(data) > 0 {
		number, kind, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		switch {
		case number == 1 && kind == protowire.VarintType:
			v, m := protowire.ConsumeVarint(data)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			total += v
			data = data[m:]
		case number == 2 && kind == protowire.BytesType:
			v, m := protowire.ConsumeBytes(data)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			label = v
			data = data[m:]
		default:
			m := protowire.ConsumeFieldValue(number, kind, data)
			if m < 0 {
				return nil, protowire.ParseError(m)
			}
			data = data[m:]
		}
	}
	var out []byte
	out = protowire.AppendTag(out, 1, protowire.VarintType)
	out = protowire.AppendVarint(out, total)
	out = protowire.AppendTag(out, 2, protowire.BytesType)
	out = protowire.AppendBytes(out, label)
	out = protowire.AppendTag(out, 3, protowire.VarintType)
	out = protowire.AppendVarint(out, protowire.EncodeZigZag(-int64(total)))
	return out, nil
}

func authorized(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	if v := md.Get("authorization"); len(v) != 1 || v[0] != "Bearer fixture" {
		return status.Error(codes.Unauthenticated, "missing token")
	}
	return nil
}

func main() {
	encoding.RegisterCodec(raw{})
	desc := grpc.ServiceDesc{
		ServiceName: "test.v1.Fixture",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "Sum", Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				if err := authorized(ctx); err != nil {
					return nil, err
				}
				var in []byte
				if err := dec(&in); err != nil {
					return nil, err
				}
				out, err := sum(in)
				if err != nil {
					return nil, status.Error(codes.InvalidArgument, err.Error())
				}
				grpc.SetTrailer(ctx, metadata.Pairs("x-checked", "protowire"))
				return &out, nil
			}},
			{MethodName: "Fail", Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				var in []byte
				if err := dec(&in); err != nil {
					return nil, err
				}
				return nil, status.Error(codes.NotFound, "no está: 100% ✓")
			}},
		},
		Streams: []grpc.StreamDesc{
			{StreamName: "Echo", ServerStreams: true, ClientStreams: true, Handler: func(srv any, stream grpc.ServerStream) error {
				if err := authorized(stream.Context()); err != nil {
					return err
				}
				count := 0
				for {
					var in []byte
					err := stream.RecvMsg(&in)
					if err == io.EOF {
						stream.SetTrailer(metadata.Pairs("x-count", fmt.Sprint(count)))
						return nil
					}
					if err != nil {
						return err
					}
					count++
					if err := stream.SendMsg(&in); err != nil {
						return err
					}
				}
			}},
		},
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(8<<20), grpc.MaxSendMsgSize(8<<20))
	server.RegisterService(&desc, struct{}{})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(listener.Addr().String())
	server.Serve(listener)
}
