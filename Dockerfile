FROM python:3.12-slim AS trainer

WORKDIR /app

RUN apt-get update && apt-get install libgomp1 -y

RUN pip install --no-cache-dir numpy lightgbm==3.3.5

COPY resources/references.json resources/references.json
COPY train_lgbm.py train_lgbm.py

RUN python3 train_lgbm.py


FROM golang:1.26 AS builder

WORKDIR /app

COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -buildvcs=false -o /app/tmp/main .


FROM alpine:3.21 as runner

WORKDIR /app
COPY --from=builder /app/.env /app/.env
COPY --from=trainer /app/testdata/lgfraud.model /app/testdata/lgfraud.model
COPY --from=builder /app/resources/normalization.json /app/resources/normalization.json
COPY --from=builder /app/resources/mcc_risk.json /app/resources/mcc_risk.json
COPY --from=builder /app/tmp/main /app/main

ENTRYPOINT ["/app/main"]