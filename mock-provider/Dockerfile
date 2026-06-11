FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN go build -trimpath -ldflags="-s -w" -o /out/mock-provider .

FROM alpine:3.20

RUN adduser -D -H -u 10001 appuser
USER appuser

COPY --from=builder /out/mock-provider /usr/local/bin/mock-provider

ENV PORT=3001
ENV DEFAULT_MODEL=mock-gpt

EXPOSE 3001
ENTRYPOINT ["/usr/local/bin/mock-provider"]
