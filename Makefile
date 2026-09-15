.PHONY: proto tidy test backend web dev-backend

export PATH := $(PATH):$(HOME)/go/bin
export AUTOSTART_CAMPAIGN ?= true
export NEXT_PUBLIC_API_URL ?= http://127.0.0.1:8080

proto:
	protoc -I . \
	  --go_out=. --go_opt=module=aperture \
	  --go-grpc_out=. --go-grpc_opt=module=aperture \
	  proto/common/v1/types.proto \
	  proto/marketdata/v1/marketdata.proto \
	  proto/trading/v1/trading.proto \
	  proto/learning/v1/learning.proto \
	  proto/sentiment/v1/sentiment.proto \
	  proto/advisor/v1/advisor.proto

tidy:
	go mod tidy

test:
	go test ./...

dev-backend:
	bash scripts/dev-backend.sh

web:
	cd apps/web && npx next dev --turbopack -p 43127 -H 0.0.0.0
