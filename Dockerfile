# syntax=docker/dockerfile:1

# --- web build stage ---------------------------------------------------------
FROM node:22-bookworm-slim AS web
WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY next.config.mjs postcss.config.js tailwind.config.ts tsconfig.json next-env.d.ts ./
COPY public ./public
COPY src ./src
RUN npm run build

# --- binary build stage ------------------------------------------------------
FROM golang:1.25 AS build
WORKDIR /src/backend

# Cache module downloads.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Build a static binary; migrations and the exported frontend are embedded via
# go:embed, so no loose SQL/HTML/assets need to be copied into the runtime image.
COPY backend/ ./
COPY --from=web /src/out ./internal/frontend/out
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/vibegrid ./cmd/vibegrid

# --- runtime stage -----------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/vibegrid /vibegrid

# No hardcoded VIBEGRID_ADDR: the binary listens on $PORT when a PaaS injects it
# (Render/Railway/Cloud Run/Koyeb), and falls back to :8081 for local/Fly.
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/vibegrid"]
