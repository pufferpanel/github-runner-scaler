FROM golang:1.24.4-alpine AS builder

COPY . .

RUN go build -o /usr/bin/github-runner-scaler github.com/pufferpanel/github-runner-scaler

FROM alpine AS final

COPY --from=builder /usr/bin/github-runner-scaler /usr/bin/

ENTRYPOINT ["/usr/bin/github-runner-scaler"]