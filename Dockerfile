<<<<<<< HEAD
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Abhängigkeiten kopieren und herunterladen
COPY go.mod go.sum ./
RUN go mod download

# Quellcode kopieren
COPY . .

# Go-Binary bauen (CGO deaktiviert für kleinere Alpine-Binaries)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o serien-tracker .

# Endgültiges Image (sehr klein)
FROM alpine:latest

# CA-Zertifikate für HTTPS (z. B. OMDb-API) hinzufügen
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Binary und Assets aus Builder-Image kopieren
COPY --from=builder /app/serien-tracker .
COPY --from=builder /app/templates ./templates/
COPY --from=builder /app/static ./static/

# Datenverzeichnis vorbereiten (für Nutzerdaten)
RUN mkdir -p data

# Port freigeben
EXPOSE 8080

# Startbefehl
CMD ["./serien-tracker"]
=======
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
COPY --from=builder /app/templates ./templates/
COPY --from=builder /app/static ./static/
RUN mkdir -p data
EXPOSE 8080
CMD ["./main"]
>>>>>>> da9a90212c2d3768187c0a975f085ff390a15a4c
