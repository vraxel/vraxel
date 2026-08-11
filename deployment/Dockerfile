# Multi-stage: UI -> Go -> distroless. Build from the repo root:
#   docker build -f deployment/Dockerfile -t vraxel-server .
FROM node:22-alpine AS ui
RUN corepack enable && corepack prepare pnpm@11.0.8 --activate
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY ui/package.json ui/
RUN pnpm install --frozen-lockfile
COPY ui/ ui/
RUN cd ui && pnpm build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/ui/dist ui/dist
ARG VERSION=docker
RUN CGO_ENABLED=0 go build \
      -ldflags "-X 'vraxel.io/vraxel/lib/buildinfo.Version=vraxel-server-${VERSION}'" \
      -o /vraxel-server vraxel.io/vraxel/app/vraxel-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /vraxel-server /vraxel-server
EXPOSE 9099
ENTRYPOINT ["/vraxel-server"]
CMD ["-config", "/etc/vraxel/config.yaml"]
