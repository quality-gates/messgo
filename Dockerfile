# Runtime image: docker build -t messgo . && docker run --rm -v "$PWD":/code messgo /code text go --ignore-tests
FROM golang:alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /app/messgo ./cmd/messgo

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /code
COPY --from=build /app/messgo /usr/local/bin/messgo
ENTRYPOINT ["messgo"]
CMD ["--help"]
