FROM golang:1.25 AS builder

WORKDIR /app

COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -buildvcs=false -o /app/tmp/main .

FROM alpine:3.21

WORKDIR /app
COPY --from=builder /app/.env /app/.env
COPY --from=builder /app/resources/references.json /app/resources/references.json
COPY --from=builder /app/resources/normalization.json /app/resources/normalization.json
COPY --from=builder /app/resources/mcc_risk.json /app/resources/mcc_risk.json
COPY --from=builder /app/tmp/main /app/main

ENTRYPOINT ["/app/main"]