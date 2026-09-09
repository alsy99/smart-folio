package grpcx

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Dial(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func MustDial(addr string) *grpc.ClientConn {
	for i := 0; i < 20; i++ {
		conn, err := Dial(addr)
		if err == nil {
			return conn
		}
		time.Sleep(250 * time.Millisecond)
	}
	conn, err := Dial(addr)
	if err != nil {
		panic(err)
	}
	return conn
}
