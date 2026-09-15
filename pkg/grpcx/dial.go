package grpcx

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Dial(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func MustDial(ctx context.Context, addr string) *grpc.ClientConn {
	var last error
	for i := 0; i < 20; i++ {
		if ctx.Err() != nil {
			panic(fmt.Errorf("dial %s: %w", addr, ctx.Err()))
		}
		conn, err := Dial(addr)
		if err == nil {
			return conn
		}
		last = err
		select {
		case <-ctx.Done():
			panic(fmt.Errorf("dial %s: %w", addr, ctx.Err()))
		case <-time.After(250 * time.Millisecond):
		}
	}
	conn, err := Dial(addr)
	if err != nil {
		if last != nil {
			panic(fmt.Errorf("dial %s: %v", addr, last))
		}
		panic(err)
	}
	return conn
}
