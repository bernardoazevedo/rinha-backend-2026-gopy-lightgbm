FROM golang:1.26 AS builder

WORKDIR /app

COPY . .
RUN go mod download

RUN go build -o ./tmp/converter ./cmd
RUN ./tmp/converter

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -buildvcs=false -o /app/tmp/main .

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/.env /app/.env
COPY --from=builder /app/graph.bin /app/graph.bin
COPY --from=builder /app/labels.json /app/labels.json
COPY --from=builder /app/resources/normalization.json /app/resources/normalization.json
COPY --from=builder /app/resources/mcc_risk.json /app/resources/mcc_risk.json
COPY --from=builder /app/tmp/main /app/main

ENTRYPOINT ["/app/main"]