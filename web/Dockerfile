# Build stage: produce static files only
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Output stage: copy dist to a minimal image for volume sharing
FROM alpine:3.20
COPY --from=builder /app/dist /dist
