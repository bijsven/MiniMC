FROM node:20-alpine AS frontend-build
WORKDIR /app/client

COPY client/package*.json ./
RUN npm ci

COPY client/ ./
RUN npm run build


FROM golang:1.23-alpine AS go-build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-build /app/client/build ./client/build

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o MiniMC .


FROM alpine:latest
WORKDIR /root/

# Install SSH client for Serveo
RUN apk add --no-cache openssh bash curl

COPY --from=go-build /app/MiniMC ./
COPY --from=go-build /app/client/build ./client/build

# Expose local ports (internal to container, Serveo will forward them)
EXPOSE 25565
EXPOSE 8080

# Copy startup script
COPY start.sh /start.sh
RUN chmod +x /start.sh

CMD ["/start.sh"]
