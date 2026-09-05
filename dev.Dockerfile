# Development image: docker build -f dev.Dockerfile -t messgo-dev . && docker run --rm -it -v "$PWD":/workspace messgo-dev
FROM golang:alpine
RUN apk add --no-cache git bash make gcc musl-dev
WORKDIR /workspace
COPY . .
CMD ["go", "test", "./..."]
