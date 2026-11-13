FROM golang:1.27.4-alpine3.24 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /notification .

FROM alpine:3.24.4
RUN addgroup -S app && adduser -S -G app app && apk add --no-cache ca-certificates
USER app
COPY --from=build /notification /notification
EXPOSE 8443
ENTRYPOINT ["/notification"]
