FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY garden /app/garden
COPY frontend/dist /app/frontend/dist
RUN mkdir -p /app/frontend/public/covers
EXPOSE 8080
ENTRYPOINT ["/app/garden"]
