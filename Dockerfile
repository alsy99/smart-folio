FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 go build -o /out/service ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/service /service
COPY campaign /app/campaign
ENV CAMPAIGN_DIR=/app/campaign/public-30d
ENTRYPOINT ["/service"]
