FROM golang:latest

ENV GO111MODULE=on \
    CGO_ENABLED=1

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

RUN go build -o main .

EXPOSE 54580

CMD ["./main", "--serve=127.0.0.1:54580", "--allow-origin=*"]
