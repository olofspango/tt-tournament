FROM golang:1.22-bullseye AS builder

RUN apt-get update && apt-get install -y libsqlite3-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /app

COPY go.mod .
RUN go mod download
COPY . .
RUN go build -o tt-tracker .

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /app/tt-tracker .
COPY --from=builder /app/static ./static
COPY --from=builder /app/data ./data

EXPOSE 8080
ENV DATABASE_PATH=/app/data/tournament.db
CMD ["./tt-tracker"]
