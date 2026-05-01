FROM golang:1.26

WORKDIR /app

RUN apt update && apt-get install -y gcc libsqlite3-dev

COPY . .
RUN go mod download

RUN go run ./cmd/main.go create

RUN CGO_ENABLED=1 GOOS=linux && go build -ldflags='-s -w' -buildvcs=false -o ./tmp/main .

ENTRYPOINT ["./tmp/main"]