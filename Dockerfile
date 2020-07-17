FROM golang:1.14-alpine as builder
ENV CGO_ENABLED=0
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /telegram ./cmd/telegram/*.go

FROM alpine:3.12
RUN apk add --no-cache ca-certificates shadow && \
    groupadd app && \
    useradd -g app app

COPY --from=builder --chown=app:app /telegram /telegram
USER app
ENTRYPOINT ["/telegram"]
