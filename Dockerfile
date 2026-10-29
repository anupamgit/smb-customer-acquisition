# Render / production image. Build from repository root:
#   docker build -t advisor .
# Requires Node 20+ for the web stage (uses bookworm node image below).

# syntax=docker/dockerfile:1

FROM node:20-bookworm AS web
WORKDIR /web
COPY src/web/package.json src/web/package-lock.json ./
RUN npm ci
COPY src/web .
RUN npm run build

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY src/api/go.mod src/api/go.sum ./
RUN go mod download
COPY src/api .
COPY --from=web /api/static ./static
RUN CGO_ENABLED=0 go build -trimpath -o /out/advisor .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/advisor /app/advisor
COPY --from=build /src/static ./static
COPY data ./data
EXPOSE 8080
CMD ["/app/advisor"]
