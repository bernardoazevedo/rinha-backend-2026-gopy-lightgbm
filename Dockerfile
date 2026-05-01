FROM golang:1.26

WORKDIR /app

COPY . .
RUN go mod download

RUN go build -o ./tmp/converter ./cmd/converter
RUN ./tmp/converter ./resources/references.json ./resources/references.bin
RUN rm -f ./resources/references.json ./resources/references.json.gz ./tmp/converter

RUN CGO_ENABLED=0 GOOS=linux && go build -ldflags='-s -w' -buildvcs=false -o ./tmp/main .

ENTRYPOINT ["./tmp/main"]