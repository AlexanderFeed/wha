FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/server/ ./cmd/server/
COPY backend/data/rektion.json ./backend/data/rektion.json
COPY --from=frontend /src/web/dist/ ./cmd/server/static/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/german-practice ./cmd/server

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/german-practice ./german-practice
COPY --from=build /src/backend/data/rektion.json ./backend/data/rektion.json
ENV PORT=8080 \
    WORDS_JSONL=/app/data/words.jsonl \
    REKTION_JSON=/app/backend/data/rektion.json
USER app
EXPOSE 8080
ENTRYPOINT ["/app/german-practice"]
