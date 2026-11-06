# ---- build ----
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/server/

# ---- runtime ----
FROM alpine:3.23
WORKDIR /app

# ffmpeg: merging video+audio. python3: runs the yt-dlp zipapp.
# deno: JavaScript runtime yt-dlp needs to solve YouTube's player challenges;
# without it YouTube returns 403s or hides formats.
# tini: reaps orphaned child processes (we run as PID 1).
RUN apk add --no-cache ca-certificates ffmpeg python3 deno curl tini

# Latest yt-dlp at build time. The server also runs `yt-dlp -U` on startup and
# periodically (YTDLP_AUTO_UPDATE), because YouTube breaks old versions fast.
RUN curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp && \
    yt-dlp --version

COPY --from=build /out/app ./app
COPY static ./static
COPY templates ./templates
COPY public ./public

ENV DOCKER_ENV=true
EXPOSE 5000
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./app"]
