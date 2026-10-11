# Victory API: a static Go binary on a small Alpine image.
# Migrations are embedded in the binary and applied on start-up.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.22
# TLS roots for Neon, Google, Chapa and Anthropic; time zones for exam times.
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 victory \
  && mkdir -p /app/uploads && chown -R victory /app
COPY --from=build /out/api /usr/local/bin/api
# Uploads land in /app/uploads only without CLOUDINARY_URL; a container's
# disk is temporary, so production should use Cloudinary.
WORKDIR /app
USER victory
ENV APP_ENV=production PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s \
  CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/api"]
