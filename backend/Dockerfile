# build API
FROM golang:1.21-alpine as build-go
RUN apk --update add ca-certificates upx && update-ca-certificates
RUN go env -w CGO_ENABLED="0"
WORKDIR /
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY *.go ./
COPY app app
RUN go build -ldflags="-s -w" -o backend . && upx --best --lzma backend

# final stage
FROM alpine:latest
# Update package list and install FFmpeg
RUN apk update && apk add --no-cache ffmpeg
RUN addgroup -S backend && adduser -S backend -G backend
USER backend
WORKDIR /home/backend
# create subdirectories
RUN mkdir -p data
COPY --from=build-go /backend ./
EXPOSE 8080 8801
ENTRYPOINT ["./backend"]