FROM golang:1.17 as build

WORKDIR /app

COPY go.* /app/

RUN go mod download

COPY . .

RUN go build -o /app/superQueueRequestRouter

# Need glibc
FROM ubuntu
COPY --from=build /app/superQueueRequestRouter /app/

COPY domain.* /app/

CMD ["/app/superQueueRequestRouter" ]
