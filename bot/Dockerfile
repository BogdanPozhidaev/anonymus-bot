FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot

FROM alpine:3.19

RUN apk add --no-cache ca-certificates imagemagick libwebp libjpeg-turbo libpng

COPY --from=builder /bot /bot

CMD ["/bot"]