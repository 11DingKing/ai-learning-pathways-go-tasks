FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ai-learning-pathways ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ai-learning-pathways /app/ai-learning-pathways
ENV APP_ADDRESS=:8080 APP_DATABASE_PATH=/data/pathways.db
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/ai-learning-pathways"]
