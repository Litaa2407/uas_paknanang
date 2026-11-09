FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gateway .
FROM alpine:3.20
COPY --from=build /gateway /gateway
EXPOSE 8000
CMD ["/gateway"]
