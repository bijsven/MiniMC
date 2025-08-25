FROM node:20-alpine AS frontend-build
WORKDIR /app/client

COPY client/package*.json ./
RUN npm i

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

RUN apk add openjdk21 ssh bash curl

COPY --from=go-build /app/MiniMC ./
COPY --from=go-build /app/client/build ./client/build

# Exposes Minecraft
EXPOSE 25565
# Exposes web portal
EXPOSE 8080

CMD ["./MiniMC"]
