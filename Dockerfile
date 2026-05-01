FROM golang:1.26

WORKDIR /app

COPY . .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux && go build -ldflags='-s -w' -buildvcs=false -o ./tmp/main .

ENTRYPOINT ["./tmp/main"]