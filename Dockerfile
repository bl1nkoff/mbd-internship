FROM golang:alpine

RUN mkdir /files
COPY server.go /files
WORKDIR /files
RUN go build -o /files/server server.go
ENTRYPOINT ["/files/server"]