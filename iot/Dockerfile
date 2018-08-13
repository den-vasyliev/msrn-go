	FROM golang:1.10 as builder

	WORKDIR /go/src/github.com/den-vasyliev/msrn-go/iot/

	ARG APP_BUILD_INFO
	ARG APP_VERSION
	ARG APP_NAME

	COPY src/main.go app.go
	RUN go get -d -v ./...
	RUN CGO_ENABLED=0 GOOS=linux go build -installsuffix cgo -a -ldflags "-X main.AppName=$APP_NAME -X main.BuildInfo=$APP_BUILD_INFO -X main.Version=$APP_VERSION" -v -o app .

	FROM alpine:latest  
	RUN apk --no-cache add ca-certificates
	WORKDIR /root/
	COPY --from=builder /go/src/github.com/den-vasyliev/msrn-go/iot/app .
	CMD ["./app"]

