FROM golang:1.16.3-alpine as builder

RUN apk add --update --no-cache make git

WORKDIR /go/src/terragrunt

# Download modules in a separate layer so docker caches it, to reduce build time
# when modules have not changed.
COPY go.mod .
COPY go.sum .
RUN go mod download -x

COPY . .

RUN CGO_ENABLED=0 make build

###

FROM scratch

COPY --from=builder /go/src/terragrunt/terragrunt /app/

ENTRYPOINT ["/app/terragrunt"]
