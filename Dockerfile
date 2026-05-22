FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/doxy .

FROM gcr.io/distroless/static-debian12:latest
COPY --from=build /out/doxy /doxy
EXPOSE 80
ENTRYPOINT ["/doxy"]
