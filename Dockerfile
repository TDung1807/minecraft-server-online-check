FROM golang:1.27.1-alpine3.23@sha256:0908ac9b9319e09d7c238aabe914e0395c51d63c4e3d0ae8c554fda9158a5769 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.23.3@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app
COPY --from=build /server /server
USER 10001:10001
ENV HTTP_ADDR=0.0.0.0:8080 METRICS_ADDR=0.0.0.0:9090
EXPOSE 8080 9090
ENTRYPOINT ["/server"]
