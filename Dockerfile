# syntax=docker/dockerfile:1

# --- build stage -------------------------------------------------------------
FROM golang:1.25 AS build
WORKDIR /src

# Cache module downloads.
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Build a static binary; migrations are embedded via go:embed, so no SQL files
# need to be copied into the runtime image.
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/vibegrid ./cmd/vibegrid

# --- runtime stage -----------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/vibegrid /vibegrid

ENV VIBEGRID_ADDR=:8081
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/vibegrid"]
