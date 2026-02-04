FROM --platform=linux/amd64 golang:1.25-alpine as builder
WORKDIR /app
COPY ./script.go .
RUN go mod init billing-killswitch && go mod tidy
RUN go build -o go_script script.go

FROM --platform=linux/amd64 alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/go_script /go_script
CMD ["/go_script"]