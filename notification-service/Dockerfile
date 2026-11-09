FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /notification .
FROM alpine:3.20
COPY --from=build /notification /notification
EXPOSE 8002
CMD ["/notification"]
